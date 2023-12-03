package grpc

import (
	"context"
	stderr "errors"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/roadrunner-server/endure/v2/dep"
	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/grpc/v4/codec"
	"github.com/roadrunner-server/grpc/v4/common"
	"github.com/roadrunner-server/grpc/v4/proxy"
	"github.com/roadrunner-server/sdk/v4/metrics"
	"github.com/roadrunner-server/sdk/v4/pool"
	"github.com/roadrunner-server/sdk/v4/state/process"
	"github.com/roadrunner-server/sdk/v4/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/health/grpc_health_v1"

	// Will register via init
	_ "google.golang.org/grpc/encoding/gzip"
)

const (
	pluginName string = "grpc"
	RrMode     string = "RR_MODE"
)

type Tracer interface {
	Tracer() *sdktrace.TracerProvider
}

type Plugin struct {
	mu           *sync.RWMutex
	config       *Config
	gPool        common.Pool
	opts         []grpc.ServerOption
	server       *grpc.Server
	rrServer     common.Server
	proxyList    []*proxy.Proxy
	healthServer *HealthCheckServer

	experimental bool

	statsExporter *metrics.StatsExporter
	tracer        *sdktrace.TracerProvider

	queueSize       prometheus.Gauge
	requestCounter  *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec

	log *zap.Logger

	// interceptors to chain
	interceptors map[string]common.Interceptor
}

func (p *Plugin) Init(cfg common.Configurer, log common.Logger, server common.Server) error {
	const op = errors.Op("grpc_plugin_init")

	if !cfg.Has(pluginName) {
		return errors.E(errors.Disabled)
	}
	// register the codec
	encoding.RegisterCodec(&codec.Codec{
		Base: encoding.GetCodec(codec.Name),
	})

	err := cfg.UnmarshalKey(pluginName, &p.config)
	if err != nil {
		return errors.E(op, err)
	}

	err = p.config.InitDefaults()
	if err != nil {
		return errors.E(op, err)
	}

	p.opts = make([]grpc.ServerOption, 0)
	p.rrServer = server
	p.proxyList = make([]*proxy.Proxy, 0, 1)

	// worker's GRPC mode
	if p.config.Env == nil {
		p.config.Env = make(map[string]string)
	}
	p.config.Env[RrMode] = pluginName

	p.log = log.NamedLogger(pluginName)
	p.mu = &sync.RWMutex{}
	p.statsExporter = newStatsExporter(p)

	// metrics
	p.queueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "requests_queue",
		Help:      "Total number of queued requests.",
	})

	p.requestCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "request_total",
		Help:      "Total number of GRPC requests processed after the server restarted, including their status codes.",
	}, []string{"grpc_method", "status_code"})

	p.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_seconds",
			Help:      "GRPC request duration.",
		},
		[]string{"grpc_method"},
	)

	p.tracer = sdktrace.NewTracerProvider()
	p.experimental = cfg.Experimental()
	p.interceptors = make(map[string]common.Interceptor)

	return nil
}

func (p *Plugin) Serve() chan error {
	const op = errors.Op("grpc_plugin_serve")
	errCh := make(chan error, 1)

	p.mu.Lock()
	defer p.mu.Unlock()

	var err error
	p.gPool, err = p.rrServer.NewPool(context.Background(), &pool.Config{
		Debug:           p.config.GrpcPool.Debug,
		Command:         p.config.GrpcPool.Command,
		NumWorkers:      p.config.GrpcPool.NumWorkers,
		MaxJobs:         p.config.GrpcPool.MaxJobs,
		AllocateTimeout: p.config.GrpcPool.AllocateTimeout,
		DestroyTimeout:  p.config.GrpcPool.DestroyTimeout,
		Supervisor:      p.config.GrpcPool.Supervisor,
	}, p.config.Env, nil)
	if err != nil {
		errCh <- errors.E(op, err)
		return errCh
	}

	p.server, err = p.createGRPCserver(p.interceptors)
	if err != nil {
		errCh <- errors.E(op, err)
		return errCh
	}

	l, err := utils.CreateListener(p.config.Listen)
	if err != nil {
		errCh <- errors.E(op, err)
		return errCh
	}

	p.healthServer = NewHeathServer(p, p.log)
	p.healthServer.RegisterServer(p.server)

	go func() {
		p.log.Info("grpc server was started", zap.String("address", p.config.Listen))

		p.healthServer.SetServingStatus(grpc_health_v1.HealthCheckResponse_SERVING)
		err = p.server.Serve(l)
		p.healthServer.Shutdown()
		if err != nil {
			// skip errors when stopping the server
			if stderr.Is(err, grpc.ErrServerStopped) {
				return
			}

			p.log.Error("grpc server was stopped", zap.Error(err))
			errCh <- errors.E(op, err)
			return
		}
	}()

	return errCh
}

func (p *Plugin) Stop(ctx context.Context) error {
	finCh := make(chan struct{}, 1)
	go func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		p.healthServer.SetServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)

		if p.server != nil {
			p.server.Stop()
		}

		p.healthServer.Shutdown()
		finCh <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-finCh:
		return nil
	}
}

func (p *Plugin) Name() string {
	return pluginName
}

func (p *Plugin) Reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.healthServer.SetServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	defer p.healthServer.SetServingStatus(grpc_health_v1.HealthCheckResponse_SERVING)

	const op = errors.Op("grpc_plugin_reset")
	p.log.Info("reset signal was received")
	// destroy old pool
	err := p.gPool.Reset(context.Background())
	if err != nil {
		return errors.E(op, err)
	}
	p.log.Info("plugin was successfully reset")

	return nil
}

func (p *Plugin) Workers() []*process.State {
	p.mu.RLock()
	defer p.mu.RUnlock()

	workers := p.gPool.Workers()

	ps := make([]*process.State, 0, len(workers))
	for i := 0; i < len(workers); i++ {
		state, err := process.WorkerProcessState(workers[i])
		if err != nil {
			return nil
		}
		ps = append(ps, state)
	}

	return ps
}

// Collects collecting grpc interceptors
func (p *Plugin) Collects() []*dep.In {
	return []*dep.In{
		dep.Fits(func(pp any) {
			interceptor := pp.(common.Interceptor)
			// just to be safe
			p.mu.Lock()
			p.interceptors[interceptor.Name()] = interceptor
			p.mu.Unlock()
		}, (*common.Interceptor)(nil)),
		dep.Fits(func(pp any) {
			p.tracer = pp.(Tracer).Tracer()
		}, (*Tracer)(nil)),
	}
}
