package grpc_test

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v6"
	"github.com/roadrunner-server/endure/v2"
	goridgeRpc "github.com/roadrunner-server/goridge/v4/pkg/rpc"
	grpcPlugin "github.com/roadrunner-server/grpc/v6"
	"github.com/roadrunner-server/logger/v6"
	"github.com/roadrunner-server/metrics/v6"
	"github.com/roadrunner-server/resetter/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/roadrunner-server/status/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	mocklogger "tests/mock"
	"tests/proto/service"
)

const getAddr = "http://127.0.0.1:2112/metrics"

type inMemoryTracer struct {
	tp  *sdktrace.TracerProvider
	exp *tracetest.InMemoryExporter
}

func newInMemoryTracer(t *testing.T) *inMemoryTracer {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return &inMemoryTracer{tp: tp, exp: exp}
}

func (m *inMemoryTracer) Init() error                      { return nil }
func (m *inMemoryTracer) Name() string                     { return "inMemoryTracer" }
func (m *inMemoryTracer) Tracer() *sdktrace.TracerProvider { return m.tp }

func TestGrpcInit(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-init.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)

	conn, err := grpc.NewClient("127.0.0.1:9091", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcOtel(t *testing.T) {
	tracer := newInMemoryTracer(t)

	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2024.2.0",
		Path:    "configs/.rr-grpc-otel.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		tracer,
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				assert.NoError(t, err)
			case <-sig:
				err = cont.Stop()
				assert.NoError(t, err)
				return
			case <-stopCh:
				err = cont.Stop()
				assert.NoError(t, err)
				return
			}
		}
	}()

	time.Sleep(time.Second)

	conn, err := grpc.NewClient("127.0.0.1:9092", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}
	wg.Wait()

	spans := tracer.exp.GetSpans()
	spanNames := make([]string, len(spans))
	for i, s := range spans {
		spanNames[i] = s.Name
	}
	require.True(t, slices.Contains(spanNames, "service.Echo/Ping"), "expected span 'service.Echo/Ping', got: %v", spanNames)
}

func TestGrpcCheckStatus(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-status.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&status.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:35544/health?plugin=grpc", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, `[{"plugin_name":"grpc","error_message":"","status_code":200}]`, string(body))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:35544/ready?plugin=grpc", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, `[{"plugin_name":"grpc","error_message":"","status_code":200}]`, string(body))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

// different services, same methods inside
func TestGrpcInitDup2(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-init-duplicate-2.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)
	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcInitMultiple(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-init-multiple.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)

	conn, err := grpc.NewClient("127.0.0.1:9001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	time.Sleep(time.Second)
	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcRqRs(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	conn, err := grpc.NewClient("127.0.0.1:9001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcFullErrorMessageIssue1193(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2024.1.0",
		Path:    "configs/.rr-grpc-rq-issue1193.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	_, err = cont.Serve()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), " If you want to be c001 you just need to contribute to 0pensource.")
}

func TestGrpcRqRsException(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-exception.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	conn, err := grpc.NewClient("127.0.0.1:9001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.Error(t, err)
	require.Equal(t, "rpc error: code = Internal desc = FOOOOOOOOOOOO", err.Error())
	require.Nil(t, resp)
	_ = conn.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcRqRsMultiple(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-multiple.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	conn, err := grpc.NewClient("127.0.0.1:9003", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)

	hc := grpc_health_v1.NewHealthClient(conn)
	hr, err := hc.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, "SERVING", hr.Status.String())

	watch, err := hc.Watch(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)

	msg := &grpc_health_v1.HealthCheckResponse{}

	err = watch.RecvMsg(msg)
	require.NoError(t, err)
	require.Equal(t, "SERVING", msg.Status.String())

	err = watch.CloseSend()
	require.NoError(t, err)
	_ = conn.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcRqRsTLS(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-tls.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	cert, err := tls.LoadX509KeyPair("test-certs/localhost+2-client.pem", "test-certs/localhost+2-client-key.pem")
	require.NoError(t, err)

	tlscfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	conn, err := grpc.NewClient("127.0.0.1:9002", grpc.WithTransportCredentials(credentials.NewTLS(tlscfg)))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

func TestGrpcRqRsTLSRootCA(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-tls-rootca.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	cert, err := tls.LoadX509KeyPair("test-certs/localhost+2-client.pem", "test-certs/localhost+2-client-key.pem")
	require.NoError(t, err)

	tlscfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	conn, err := grpc.NewClient("127.0.0.1:9003", grpc.WithTransportCredentials(credentials.NewTLS(tlscfg)))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}
	wg.Wait()
}

func TestGrpcRqRsTLS_WithReset(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-tls.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		&resetter.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	cert, err := tls.LoadX509KeyPair("test-certs/localhost+2-client.pem", "test-certs/localhost+2-client-key.pem")
	require.NoError(t, err)

	tlscfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	conn, err := grpc.NewClient("127.0.0.1:9002", grpc.WithTransportCredentials(credentials.NewTLS(tlscfg)))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)

	// reset
	t.Run("SendReset", sendReset("127.0.0.1:6009"))

	resp2, err2 := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err2)
	require.Equal(t, "TOST", resp2.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}
	wg.Wait()
}

func TestGRPCMetrics(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-metrics.yaml",
	}

	l, oLogger := mocklogger.SlogTestLogger(slog.LevelDebug)
	err := cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&grpcPlugin.Plugin{},
		&metrics.Plugin{},
		l,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	tt := time.NewTimer(time.Minute * 3)
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer tt.Stop()
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-tt.C:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)

	conn, err := grpc.NewClient("127.0.0.1:9005", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)

	time.Sleep(time.Millisecond * 500)
	genericOut, err := get()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, `rr_grpc_workers_memory_bytes`)
	assert.Contains(t, genericOut, `rr_grpc_worker_state`)
	assert.Contains(t, genericOut, `rr_grpc_worker_memory_bytes`)
	assert.Contains(t, genericOut, `rr_grpc_request_duration_seconds`)
	assert.Contains(t, genericOut, `rr_grpc_request_total`)
	assert.Contains(t, genericOut, `rr_grpc_requests_queue`)

	_ = conn.Close()
	close(sig)
	wg.Wait()

	require.Equal(t, 1, oLogger.FilterMessageSnippet("grpc server was started").Len())
	require.Equal(t, 1, oLogger.FilterMessageSnippet("method was called successfully").Len())
}

func sendReset(address string) func(t *testing.T) {
	return func(t *testing.T) {
		var d net.Dialer
		conn, err := d.DialContext(context.Background(), "tcp", address)
		assert.NoError(t, err)
		client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
		// WorkerList contains list of workers.

		var ret bool
		err = client.Call("resetter.Reset", "grpc", &ret)
		assert.NoError(t, err)
		assert.True(t, ret)
		ret = false

		var services []string
		err = client.Call("resetter.List", nil, &services)
		assert.NotNil(t, services)
		assert.NoError(t, err)
		require.Equal(t, []string{"grpc"}, services)
	}
}

// get request and return body
func get() (string, error) {
	r, err := http.Get(getAddr) //nolint:noctx
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	err = r.Body.Close()
	if err != nil {
		return "", err
	}
	// unsafe
	return string(b), err
}

func Test_GrpcRqOtlp(t *testing.T) {
	tracer := newInMemoryTracer(t)

	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-otlp.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		tracer,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 1)

	conn, err := grpc.NewClient("127.0.0.1:9001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "TOST"})
	require.NoError(t, err)
	require.Equal(t, "TOST", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}
	wg.Wait()

	spans := tracer.exp.GetSpans()
	spanNames := make([]string, len(spans))
	for i, s := range spans {
		spanNames[i] = s.Name
	}
	require.True(t, slices.Contains(spanNames, "service.Echo/Ping"), "expected span 'service.Echo/Ping', got: %v", spanNames)
}
