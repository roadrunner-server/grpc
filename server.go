package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/grpc/v5/api"
	"github.com/roadrunner-server/grpc/v5/parser"
	"github.com/roadrunner-server/grpc/v5/proxy"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

func (p *Plugin) createGRPCserver(interceptors map[string]api.Interceptor) (*grpc.Server, error) {
	const op = errors.Op("grpc_plugin_create_server")
	opts, err := p.serverOptions()
	if err != nil {
		return nil, errors.E(op, err)
	}

	unaryInterceptors := []grpc.UnaryServerInterceptor{
		grpc.UnaryServerInterceptor(p.interceptor),
	}

	for _, interceptor := range interceptors {
		unaryInterceptors = append(
			unaryInterceptors,
			interceptor.UnaryServerInterceptor(),
		)
	}

	opts = append(
		opts,
		grpc.ChainUnaryInterceptor(
			unaryInterceptors...,
		),
	)

	opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(p.tracer), otelgrpc.WithPropagators(p.prop))))
	server := grpc.NewServer(opts...)

	for i := range p.config.Proto {
		if p.config.Proto[i] == "" {
			continue
		}

		// php proxy services
		services, errP := parser.File(p.config.Proto[i], path.Dir(p.config.Proto[i]))
		if errP != nil {
			return nil, errP
		}

		for _, service := range services {
			px := proxy.NewProxy(fmt.Sprintf("%s.%s", service.Package, service.Name), p.config.Proto[i], p.log.Named(service.Name), p.gPool, p.mu, p.prop)
			for _, m := range service.Methods {
				px.RegisterMethod(m.Name)
			}

			server.RegisterService(px.ServiceDesc(), px)
			p.proxyList = append(p.proxyList, px)
		}
	}

	return server, nil
}

func (p *Plugin) interceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()

	p.queueSize.Inc()

	resp, err := handler(ctx, req)

	s, ok := status.FromError(err)
	var statusCode codes.Code
	switch ok {
	case true:
		statusCode = s.Code()
	case false:
		statusCode = status.New(codes.Unknown, err.Error()).Code()
	}

	defer func() {
		p.requestCounter.WithLabelValues(info.FullMethod, statusCode.String()).Inc()
		p.requestDuration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		p.queueSize.Dec()
	}()

	if err != nil {
		p.log.Error("method call was finished with error", zap.Error(err), zap.String("method", info.FullMethod), zap.Time("start", start), zap.Int64("elapsed", time.Since(start).Milliseconds()))

		return nil, err
	}

	p.log.Debug("method was called successfully", zap.String("method", info.FullMethod), zap.Time("start", start), zap.Int64("elapsed", time.Since(start).Milliseconds()))
	return resp, nil
}

func (p *Plugin) serverOptions() ([]grpc.ServerOption, error) {
	const op = errors.Op("grpc_plugin_server_options")

	var tcreds credentials.TransportCredentials
	var opts []grpc.ServerOption
	var cert tls.Certificate
	var certPool *x509.CertPool
	var rca []byte
	var err error

	if p.config.EnableTLS() {
		// if client CA is not empty, we combine it with Cert and Key
		if p.config.TLS.RootCA != "" {
			cert, err = tls.LoadX509KeyPair(p.config.TLS.Cert, p.config.TLS.Key)
			if err != nil {
				return nil, err
			}

			certPool, err = x509.SystemCertPool()
			if err != nil {
				return nil, err
			}
			if certPool == nil {
				certPool = x509.NewCertPool()
			}

			rca, err = os.ReadFile(p.config.TLS.RootCA)
			if err != nil {
				return nil, err
			}

			if ok := certPool.AppendCertsFromPEM(rca); !ok {
				return nil, errors.E(op, errors.Str("could not append Certs from PEM"))
			}

			opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{
				MinVersion:   tls.VersionTLS12,
				ClientAuth:   p.config.TLS.auth,
				Certificates: []tls.Certificate{cert},
				ClientCAs:    certPool,
			})))
		} else {
			// regular TLS from the cert+key
			tcreds, err = credentials.NewServerTLSFromFile(p.config.TLS.Cert, p.config.TLS.Key)
			if err != nil {
				return nil, err
			}

			opts = append(opts, grpc.Creds(tcreds))
		}
	}

	serverOptions := []grpc.ServerOption{
		grpc.MaxSendMsgSize(int(p.config.MaxSendMsgSize)),
		grpc.MaxRecvMsgSize(int(p.config.MaxRecvMsgSize)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     p.config.MaxConnectionIdle,
			MaxConnectionAge:      p.config.MaxConnectionAge,
			MaxConnectionAgeGrace: p.config.MaxConnectionAge,
			Time:                  p.config.PingTime,
			Timeout:               p.config.Timeout,
		}),
		grpc.MaxConcurrentStreams(uint32(p.config.MaxConcurrentStreams)), //nolint:gosec
	}

	opts = append(opts, serverOptions...)
	opts = append(opts, p.opts...)

	// custom codec is required to bypass protobuf, a common interceptor used for debug and stats
	return opts, nil
}
