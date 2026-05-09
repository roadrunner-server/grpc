package interceptor1

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/roadrunner-server/grpc/v6/api"
	"google.golang.org/grpc"
)

const (
	name         = "interceptor1"
	MarkerPrefix = "interceptor1-marker-"
)

type Logger interface {
	NamedLogger(name string) *slog.Logger
}

type Plugin struct {
	log *slog.Logger
}

var _ api.Interceptor = (*Plugin)(nil)

func (p *Plugin) Init(logger Logger) error {
	p.log = logger.NamedLogger(name)
	return nil
}

type markerKey struct{}

var markerCtxKey = markerKey{} //nolint:gochecknoglobals

func MarkerFromContext(ctx context.Context) (string, bool) {
	marker, ok := ctx.Value(markerCtxKey).(string)
	return marker, ok
}

func (p *Plugin) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		marker := fmt.Sprintf("%s%d", MarkerPrefix, time.Now().UnixNano())
		p.logger().Info("interceptor1 created marker", "method", info.FullMethod, "marker", marker)

		return handler(context.WithValue(ctx, markerCtxKey, marker), req)
	}
}

func (p *Plugin) Name() string {
	return name
}

func (p *Plugin) logger() *slog.Logger {
	if p.log == nil {
		return slog.New(slog.DiscardHandler)
	}

	return p.log
}
