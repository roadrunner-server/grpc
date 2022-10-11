package grpc

import (
	"context"
	stderr "errors"
	"sync"

	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/grpc/v3/codec"
	"github.com/roadrunner-server/grpc/v3/proxy"
	"github.com/roadrunner-server/sdk/v3/metrics"
	"github.com/roadrunner-server/sdk/v3/payload"
	"github.com/roadrunner-server/sdk/v3/pool"
	staticPool "github.com/roadrunner-server/sdk/v3/pool/static_pool"
	"github.com/roadrunner-server/sdk/v3/state/process"
	"github.com/roadrunner-server/sdk/v3/utils"
	"github.com/roadrunner-server/sdk/v3/worker"
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

type Configurer interface {
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error
	// Has checks if config section exists.
	Has(name string) bool
}

type Pool interface {
	// Workers returns worker list associated with the pool.
	Workers() (workers []*worker.Process)

	// Exec payload
	Exec(ctx context.Context, p *payload.Payload) (*payload.Payload, error)

	// Reset kill all workers inside the watcher and replaces with new
	Reset(ctx context.Context) error

	// Destroy all underlying stack (but let them complete the task).
	Destroy(ctx context.Context)
}

// Server creates workers for the application.
type Server interface {
	NewPool(ctx context.Context, cfg *pool.Config, env map[string]string, _ *zap.Logger) (*staticPool.Pool, error)
}

type Plugin struct {
	mu            *sync.RWMutex
	config        *Config
	gPool         Pool
	opts          []grpc.ServerOption
	server        *grpc.Server
	rrServer      Server
	proxyList     []*proxy.Proxy
	healthServer  *HealthCheckServer
	statsExporter *metrics.StatsExporter

	log *zap.Logger
}

func (p *Plugin) Init(cfg Configurer, log *zap.Logger, server Server) error {
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

	p.log = new(zap.Logger)
	*p.log = *log
	p.mu = &sync.RWMutex{}
	p.statsExporter = newStatsExporter(p)

	return nil
}

func (p *Plugin) Serve() chan error {
	const op = errors.Op("grpc_plugin_serve")
	errCh := make(chan error, 1)

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

	p.server, err = p.createGRPCserver()
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

func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.healthServer.SetServingStatus(grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	if p.server != nil {
		p.server.Stop()
	}

	p.healthServer.Shutdown()
	return nil
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
