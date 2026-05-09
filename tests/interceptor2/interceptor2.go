package interceptor2

import (
	"context"
	"log/slog"

	"github.com/roadrunner-server/grpc/v6/api"
	"google.golang.org/grpc"
	"tests/interceptor1"
)

const name = "interceptor2"

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

func (p *Plugin) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		marker, ok := interceptor1.MarkerFromContext(ctx)
		if ok {
			p.logger().Info("interceptor2 received marker", "method", info.FullMethod, "marker", marker)
		} else {
			panic("interceptor2 did not receive marker from interceptor1, context is not properly propagated")
		}

		return handler(ctx, req)
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
