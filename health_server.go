package grpc

import (
	"context"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type HealthCheckServer struct {
	mu sync.Mutex
	grpc_health_v1.HealthCheckRequest
	plugin   *Plugin
	log      *zap.Logger
	shutdown bool
	updates  map[grpc_health_v1.Health_WatchServer]chan grpc_health_v1.HealthCheckResponse_ServingStatus
	status   grpc_health_v1.HealthCheckResponse_ServingStatus
}

func NewHeathServer(p *Plugin, log *zap.Logger) *HealthCheckServer {
	return &HealthCheckServer{
		updates: make(map[grpc_health_v1.Health_WatchServer]chan grpc_health_v1.HealthCheckResponse_ServingStatus, 1),
		plugin:  p,
		log:     log,
		status:  grpc_health_v1.HealthCheckResponse_NOT_SERVING,
	}
}

// List provides a non-atomic snapshot of the health of all the available
// services.
//
// The server may respond with a RESOURCE_EXHAUSTED error if too many services
// exist.
//
// Clients should set a deadline when calling List, and can declare the server
// unhealthy if they do not receive a timely response.
//
// Clients should keep in mind that the list of health services exposed by an
// application can change over the lifetime of the process.
func (h *HealthCheckServer) List(context.Context, *grpc_health_v1.HealthListRequest) (*grpc_health_v1.HealthListResponse, error) {
	return &grpc_health_v1.HealthListResponse{
		Statuses: map[string]*grpc_health_v1.HealthCheckResponse{
			"grpc": {
				Status: h.status,
			},
		},
	}, nil
}

func (h *HealthCheckServer) Check(_ context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: h.status,
	}, nil
}

func (h *HealthCheckServer) Watch(_ *grpc_health_v1.HealthCheckRequest, stream grpc_health_v1.Health_WatchServer) error {
	update := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus, 1)
	h.mu.Lock()

	// put the initial status
	update <- h.status
	h.updates[stream] = update

	defer func() {
		h.mu.Lock()
		delete(h.updates, stream)
		h.mu.Unlock()
	}()

	h.mu.Unlock()

	var lastStatus grpc_health_v1.HealthCheckResponse_ServingStatus = -1

	for {
		select {
		case servingStatus := <-update:
			if lastStatus == servingStatus {
				h.log.Debug("status won't changed", zap.String("status", lastStatus.String()))
				continue
			}
			lastStatus = servingStatus

			err := stream.Send(&grpc_health_v1.HealthCheckResponse{Status: servingStatus})
			if err != nil {
				return status.Error(codes.Canceled, "Stream has ended")
			}
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "Stream has ended")
		}
	}
}

func (h *HealthCheckServer) SetServingStatus(servingStatus grpc_health_v1.HealthCheckResponse_ServingStatus) {
	h.mu.Lock()
	if h.shutdown {
		h.log.Info("health status changing is ignored, because health service is shutdown")
		return
	}
	h.status = servingStatus

	// clear non relevant statuses
	for _, upd := range h.updates {
		select {
		case <-upd:
		default:
		}

		// put the most recent one
		upd <- servingStatus
	}
	h.mu.Unlock()
}

func (h *HealthCheckServer) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.shutdown = true

	for _, upd := range h.updates {
		select {
		case <-upd:
		default:
		}
	}
}

func (h *HealthCheckServer) RegisterServer(serv *grpc.Server) {
	grpc_health_v1.RegisterHealthServer(serv, h)
}
