package grpc_test

import (
	"io"
	"net/http"
	"slices"
	"testing"

	"tests/helpers"

	"github.com/roadrunner-server/metrics/v6"
	"github.com/roadrunner-server/status/v6"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const (
	metricsAddr     = "127.0.0.1:2112"
	statusAddr      = "127.0.0.1:35544"
	metricsGRPCAddr = "127.0.0.1:9005"
	otelAddr        = "127.0.0.1:9092"
	otlpAddr        = "127.0.0.1:9001"
)

// inMemoryTracer stands in for the otel plugin, collecting spans in memory so a
// test can assert on them without a collector.
type inMemoryTracer struct {
	tp  *sdktrace.TracerProvider
	exp *tracetest.InMemoryExporter
}

func newInMemoryTracer(t *testing.T) *inMemoryTracer {
	t.Helper()

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	return &inMemoryTracer{tp: tp, exp: exp}
}

func (m *inMemoryTracer) Init() error                      { return nil }
func (m *inMemoryTracer) Name() string                     { return "inMemoryTracer" }
func (m *inMemoryTracer) Tracer() *sdktrace.TracerProvider { return m.tp }

// spanNames returns the names of every span collected so far.
func (m *inMemoryTracer) spanNames() []string {
	spans := m.exp.GetSpans()
	names := make([]string, len(spans))
	for i := range spans {
		names[i] = spans[i].Name
	}
	return names
}

// get fetches a plain http endpoint and returns its status and body.
func get(t *testing.T, url string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer func() { require.NoError(t, resp.Body.Close()) }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

// TestWorkerMetricsAreExported drives one call and then checks the pool gauges
// the plugin registers reach the exporter.
func TestWorkerMetricsAreExported(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-grpc-metrics.yaml",
		append(grpcPlugins(), &metrics.Plugin{}),
		helpers.WithTCPProbe(metricsGRPCAddr),
	)

	got, err := ping(t, helpers.Dial(t, metricsGRPCAddr), "TOST")
	require.NoError(t, err)
	require.Equal(t, "TOST", got)

	code, body := get(t, "http://"+metricsAddr+"/metrics")

	require.Equal(t, http.StatusOK, code)
	for _, want := range []string{
		"rr_grpc_workers_memory_bytes",
		"rr_grpc_worker_state",
		"rr_grpc_worker_memory_bytes",
	} {
		require.Contains(t, body, want)
	}
}

// TestStatusEndpoints covers health and ready for the grpc plugin, plus a name
// that is not registered.
func TestStatusEndpoints(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-grpc-status.yaml",
		append(grpcPlugins(), &status.Plugin{}),
		helpers.WithTCPProbe(statusAddr),
	)

	const healthy = `[{"plugin_name":"grpc","error_message":"","status_code":200}]`

	for _, path := range []string{"/health?plugin=grpc", "/ready?plugin=grpc"} {
		t.Run(path, func(t *testing.T) {
			code, body := get(t, "http://"+statusAddr+path)

			require.Equal(t, http.StatusOK, code)
			require.JSONEq(t, healthy, body)
		})
	}

	t.Run("unknown plugin reports nothing", func(t *testing.T) {
		code, body := get(t, "http://"+statusAddr+"/health?plugin=not-registered")

		require.Equal(t, http.StatusOK, code)
		require.JSONEq(t, `[]`, body)
	})
}

// TestOtelSpanIsRecorded checks the plugin opens a span named after the called
// method.
func TestOtelSpanIsRecorded(t *testing.T) {
	tracer := newInMemoryTracer(t)

	helpers.Start(t,
		"configs/.rr-grpc-otel.yaml",
		append(grpcPlugins(), tracer),
		helpers.WithTCPProbe(otelAddr),
	)

	got, err := ping(t, helpers.Dial(t, otelAddr), "TOST")
	require.NoError(t, err)
	require.Equal(t, "TOST", got)

	require.True(t, slices.Contains(tracer.spanNames(), "service.Echo/Ping"),
		"expected a span for the called method, got: %v", tracer.spanNames())
}

// TestOtlpSpanIsRecorded is the same over the otlp-configured server.
func TestOtlpSpanIsRecorded(t *testing.T) {
	tracer := newInMemoryTracer(t)

	helpers.Start(t,
		"configs/.rr-grpc-rq-otlp.yaml",
		append(grpcPlugins(), tracer),
		helpers.WithTCPProbe(otlpAddr),
	)

	got, err := ping(t, helpers.Dial(t, otlpAddr), "TOST")
	require.NoError(t, err)
	require.Equal(t, "TOST", got)

	require.True(t, slices.Contains(tracer.spanNames(), "service.Echo/Ping"),
		"expected a span for the called method, got: %v", tracer.spanNames())
}
