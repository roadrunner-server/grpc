package grpc_test

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v6"
	"github.com/roadrunner-server/endure/v2"
	grpcPlugin "github.com/roadrunner-server/grpc/v6"
	"github.com/roadrunner-server/logger/v6"
	protoreg "github.com/roadrunner-server/protoreg/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpcreflectv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

// TestGrpcReflection boots the gRPC server together with the protoreg plugin and
// verifies that server reflection lists the dynamically proxied service and
// serves its full file descriptor sourced from the protoreg registry.
func TestGrpcReflection(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2023.3.0",
		Path:    "configs/.rr-grpc-reflection.yaml",
	}

	err := cont.RegisterAll(
		cfg,
		&grpcPlugin.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&server.Plugin{},
		&protoreg.Plugin{},
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

	stopCh := make(chan struct{}, 1)

	wg.Go(func() {
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
	})

	time.Sleep(time.Second)

	conn, err := grpc.NewClient("127.0.0.1:9099", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	require.NotNil(t, conn)

	assertReflection(t, conn)

	_ = conn.Close()

	stopCh <- struct{}{}

	wg.Wait()
}

// assertReflection drives the v1 server-reflection API: it lists the registered
// services, then resolves the file descriptor that contains the proxied
// service. A non-empty descriptor proves the protoreg registry is actually
// backing reflection (plain reflection over the dynamic proxy could not serve
// the file descriptors).
func assertReflection(t *testing.T, conn *grpc.ClientConn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	client := grpcreflectv1.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	require.NoError(t, err)

	err = stream.Send(&grpcreflectv1.ServerReflectionRequest{
		MessageRequest: &grpcreflectv1.ServerReflectionRequest_ListServices{ListServices: "*"},
	})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)

	services := resp.GetListServicesResponse().GetService()
	names := make([]string, 0, len(services))
	for _, svc := range services {
		names = append(names, svc.GetName())
	}
	require.Contains(t, names, "service.Echo")

	err = stream.Send(&grpcreflectv1.ServerReflectionRequest{
		MessageRequest: &grpcreflectv1.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: "service.Echo"},
	})
	require.NoError(t, err)

	resp, err = stream.Recv()
	require.NoError(t, err)

	fdResp := resp.GetFileDescriptorResponse()
	require.NotNil(t, fdResp)
	require.NotEmpty(t, fdResp.GetFileDescriptorProto())
}
