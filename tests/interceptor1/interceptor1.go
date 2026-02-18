package interceptor1

import (
	"context"
	"fmt"
	"time"

	"github.com/roadrunner-server/grpc/v5/api"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const (
	name         = "interceptor1"
	MarkerPrefix = "interceptor1-marker-"
)

type Logger interface {
	NamedLogger(name string) *zap.Logger
}

type Plugin struct {
	log *zap.Logger
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
		p.logger().Info("interceptor1 created marker", zap.String("method", info.FullMethod), zap.String("marker", marker))

		return handler(context.WithValue(ctx, markerCtxKey, marker), req)
	}
}

func (p *Plugin) Name() string {
	return name
}

func (p *Plugin) logger() *zap.Logger {
	if p.log == nil {
		return zap.NewNop()
	}

	return p.log
}
