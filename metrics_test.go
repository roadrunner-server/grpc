package grpc

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/roadrunner-server/pool/v2/fsm"
	"github.com/roadrunner-server/pool/v2/state/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeInformer returns a fixed set of worker states so the StatsExporter can be
// exercised without a running worker pool.
type fakeInformer struct {
	states []*process.State
}

func (f *fakeInformer) Workers() []*process.State { return f.states }

func TestStatsExporter_DescribeAndCollect(t *testing.T) {
	inf := &fakeInformer{states: []*process.State{
		{Pid: 1, Status: fsm.StateReady, StatusStr: "ready", MemoryUsage: 100},
		{Pid: 2, Status: fsm.StateWorking, StatusStr: "working", MemoryUsage: 200},
		{Pid: 3, Status: fsm.StateErrored, StatusStr: "errored", MemoryUsage: 300},
	}}

	exp := newStatsExporter(inf)

	// Describe must announce every descriptor the collector can emit.
	descCh := make(chan *prometheus.Desc, 16)
	exp.Describe(descCh)
	assert.Len(t, descCh, 7, "StatsExporter must describe all 7 descriptors")

	// Collect through a registry so the metric families can be asserted by name.
	reg := prometheus.NewRegistry()
	require.NoError(t, reg.Register(exp))

	mfs, err := reg.Gather()
	require.NoError(t, err)

	byName := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		byName[mf.GetName()] = mf
	}

	// One worker lands in each branch of Collect's switch: ready / working /
	// everything-else (StateErrored falls through to the invalid bucket).
	assert.Equal(t, float64(1), gaugeValue(t, byName, "rr_grpc_workers_ready"))
	assert.Equal(t, float64(1), gaugeValue(t, byName, "rr_grpc_workers_working"))
	assert.Equal(t, float64(1), gaugeValue(t, byName, "rr_grpc_workers_invalid"))

	// Totals: 3 workers, cumulative RSS = 100 + 200 + 300.
	assert.Equal(t, float64(3), gaugeValue(t, byName, "rr_grpc_total_workers"))
	assert.Equal(t, float64(600), gaugeValue(t, byName, "rr_grpc_workers_memory_bytes"))

	// Per-worker series: one sample per worker for state and memory.
	require.Contains(t, byName, "rr_grpc_worker_state")
	require.Contains(t, byName, "rr_grpc_worker_memory_bytes")
	assert.Len(t, byName["rr_grpc_worker_state"].GetMetric(), 3)
	assert.Len(t, byName["rr_grpc_worker_memory_bytes"].GetMetric(), 3)
}

// gaugeValue returns the single gauge sample for a label-less metric family.
func gaugeValue(t *testing.T, byName map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	mf, ok := byName[name]
	require.Truef(t, ok, "metric %q was not collected", name)
	require.Lenf(t, mf.GetMetric(), 1, "metric %q must have a single sample", name)
	return mf.GetMetric()[0].GetGauge().GetValue()
}

func TestMetricsCollector(t *testing.T) {
	p := &Plugin{
		statsExporter:   newStatsExporter(&fakeInformer{}),
		queueSize:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "q"}),
		requestCounter:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "c"}, []string{"l"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "d"}, []string{"l"}),
	}

	assert.Len(t, p.MetricsCollector(), 4)
}
