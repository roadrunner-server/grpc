package grpc

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// fakeWatchStream is a minimal grpc_health_v1.Health_WatchServer: it records the
// responses Watch sends and exposes a controllable context. Only Send and
// Context are used by HealthCheckServer.Watch; the embedded grpc.ServerStream
// satisfies the rest of the interface and is intentionally left nil.
type fakeWatchStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *grpc_health_v1.HealthCheckResponse
}

func (f *fakeWatchStream) Context() context.Context { return f.ctx }

func (f *fakeWatchStream) Send(resp *grpc_health_v1.HealthCheckResponse) error {
	f.sent <- resp
	return nil
}

func TestHealthServer_CheckListAndServingStatus(t *testing.T) {
	h := NewHeathServer(nil, discardLog())

	// A fresh server starts NOT_SERVING.
	resp, err := h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, resp.GetStatus())

	// List reports the same status under the "grpc" key.
	lst, err := h.List(t.Context(), &grpc_health_v1.HealthListRequest{})
	require.NoError(t, err)
	require.Contains(t, lst.GetStatuses(), "grpc")
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, lst.GetStatuses()["grpc"].GetStatus())

	// Flipping to SERVING is reflected by Check.
	h.SetServingStatus(grpc_health_v1.HealthCheckResponse_SERVING)
	resp, err = h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.GetStatus())
}

func TestHealthServer_ShutdownIgnoresStatusChange(t *testing.T) {
	h := NewHeathServer(nil, discardLog())
	h.SetServingStatus(grpc_health_v1.HealthCheckResponse_SERVING)

	h.Shutdown()

	// After Shutdown, further status changes must be ignored.
	h.SetServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	resp, err := h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.GetStatus(),
		"status must not change after Shutdown")
}

func TestHealthServer_Watch(t *testing.T) {
	h := NewHeathServer(nil, discardLog())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	stream := &fakeWatchStream{
		ctx:  ctx,
		sent: make(chan *grpc_health_v1.HealthCheckResponse, 8),
	}

	watchErr := make(chan error, 1)
	go func() { watchErr <- h.Watch(&grpc_health_v1.HealthCheckRequest{}, stream) }()

	// Watch emits the current status immediately on subscribe.
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, recvStatus(t, stream.sent))

	// A status change is streamed to the watcher.
	h.SetServingStatus(grpc_health_v1.HealthCheckResponse_SERVING)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, recvStatus(t, stream.sent))

	// Canceling the stream context ends Watch with a Canceled status.
	cancel()

	select {
	case err := <-watchErr:
		require.Error(t, err)
		assert.Equal(t, codes.Canceled, status.Code(err))
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not return after context cancellation")
	}
}

func recvStatus(t *testing.T, ch <-chan *grpc_health_v1.HealthCheckResponse) grpc_health_v1.HealthCheckResponse_ServingStatus {
	t.Helper()
	select {
	case resp := <-ch:
		return resp.GetStatus()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a health status update")
		return 0
	}
}

func TestHealthServer_RegisterServer(t *testing.T) {
	h := NewHeathServer(nil, discardLog())
	srv := grpc.NewServer()
	t.Cleanup(srv.Stop)

	// RegisterServer must wire the health service onto the gRPC server.
	require.NotPanics(t, func() { h.RegisterServer(srv) })
}
