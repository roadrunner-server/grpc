package grpc_test

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"tests/interceptor1"
	"tests/interceptor2"
	mocklogger "tests/mock"
	"tests/proto/service"

	"github.com/roadrunner-server/config/v5"
	"github.com/roadrunner-server/endure/v2"
	grpcPlugin "github.com/roadrunner-server/grpc/v5"
	rpcPlugin "github.com/roadrunner-server/rpc/v5"
	"github.com/roadrunner-server/server/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGrpcInterceptors(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-rq-interceptors.yaml",
	}

	l, observed := mocklogger.ZapTestLogger(zap.DebugLevel)
	err := cont.RegisterAll(
		cfg,
		&interceptor1.Plugin{},
		&interceptor2.Plugin{},
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&server.Plugin{},
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

	conn, err := grpc.NewClient("127.0.0.1:9011", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	client := service.NewEchoClient(conn)
	resp, err := client.Ping(context.Background(), &service.Message{Msg: "hello"})
	require.NoError(t, err)
	require.Equal(t, "HELLO", resp.Msg)
	_ = conn.Close()

	stopCh <- struct{}{}
	wg.Wait()

	interceptor1Log := observed.FilterMessageSnippet("interceptor1 created marker")
	require.Equal(t, 1, interceptor1Log.Len())

	interceptor2Log := observed.FilterMessageSnippet("interceptor2 received marker")
	require.Equal(t, 1, interceptor2Log.Len())

	marker, ok := interceptor2Log.All()[0].ContextMap()["marker"].(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(marker, interceptor1.MarkerPrefix))
}
