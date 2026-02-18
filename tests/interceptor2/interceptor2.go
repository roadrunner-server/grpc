package interceptor2

import (
	"context"

	"tests/interceptor1"

	"github.com/roadrunner-server/grpc/v5/api"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const name = "interceptor2"

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

func (p *Plugin) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		marker, ok := interceptor1.MarkerFromContext(ctx)
		if ok {
			p.logger().Info("interceptor2 received marker", zap.String("method", info.FullMethod), zap.String("marker", marker))
		} else {
			panic("interceptor2 did not receive marker from interceptor1, context is not properly propagated")
		}

		return handler(ctx, req)
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
