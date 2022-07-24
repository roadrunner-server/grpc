package grpc

import (
	"context"
	stderr "errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/roadrunner-server/api/v2/plugins/config"
	"github.com/roadrunner-server/api/v2/plugins/server"
	"github.com/roadrunner-server/api/v2/pool"
	"github.com/roadrunner-server/api/v2/state/process"
	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/grpc/v2/codec"
	"github.com/roadrunner-server/grpc/v2/proxy"
	"github.com/roadrunner-server/sdk/v2/metrics"
	poolImpl "github.com/roadrunner-server/sdk/v2/pool"
	processImpl "github.com/roadrunner-server/sdk/v2/state/process"
	"github.com/roadrunner-server/sdk/v2/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	// Will register via init
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/reflection"
)

const (
	pluginName string = "grpc"
	RrMode     string = "RR_MODE"
)

type Plugin struct {
	mu            *sync.RWMutex
	config        *Config
	gPool         pool.Pool
	opts          []grpc.ServerOption
	server        *grpc.Server
	rrServer      server.Server
	proxyList     []*proxy.Proxy
	healthServer  *HealthCheckServer
	statsExporter *metrics.StatsExporter

	log *zap.Logger
}

func (p *Plugin) Init(cfg config.Configurer, log *zap.Logger, server server.Server) error {
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
	p.gPool, err = p.rrServer.NewWorkerPool(context.Background(), &poolImpl.Config{
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

	// register reflection server
	// doc: https://github.com/grpc/grpc-go/blob/master/Documentation/server-reflection-tutorial.md
	if p.config.EnableReflectionServer {
		reflection.Register(p.server)
		// register proto descriptions manually
		err = registerProtoFile(p.config.Proto, p.log)
		if err != nil {
			errCh <- err
			return errCh
		}
	}

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
		state, err := processImpl.WorkerProcessState(workers[i])
		if err != nil {
			return nil
		}
		ps = append(ps, state)
	}

	return ps
}

func registerProtoFile(protofiles []string, log *zap.Logger) error {
	// panic handler
	defer func() {
		// panic handler, RR tried to register a file which already registered
		if r := recover(); r != nil {
			globalFiles := make([]string, 0, protoregistry.GlobalFiles.NumFiles())
			protoregistry.GlobalFiles.RangeFiles(func(desc protoreflect.FileDescriptor) bool {
				if desc.FullName() != "" {
					globalFiles = append(globalFiles, desc.Path())
				}
				return true
			})

			searchedBy := make([]string, 0, len(protofiles))
			for i := 0; i < len(protofiles); i++ {
				searchedBy = append(searchedBy, filepath.Base(protofiles[i]))
			}

			log.Error("attempted to register a duplicate", zap.Strings("protofiles", searchedBy), zap.Strings("global_registry", globalFiles))
		}
	}()

	for i := 0; i < len(protofiles); i++ {
		// get absolute path to the file
		absPath, err := filepath.Abs(filepath.Dir(protofiles[i]))
		if err != nil {
			return err
		}

		fileName := filepath.Base(protofiles[i])
		// check if we already have the file registered
		// we use filename here, because we don't use these protos inside golang app
		// and we register them by its name
		_, err = protoregistry.GlobalFiles.FindFileByPath(fileName)
		// it's ok if file not found, we need to register it
		if err != nil && !stderr.Is(err, protoregistry.NotFound) {
			return err
		}
		// we should avoid registering duplicates, that leads to panic
		// if err is eq to nil, than we found a file and should avoid to double-register it
		if err == nil {
			continue
		}

		// save the file in the temp: /tmp/fileName_tmp.pb
		tmpFile := filepath.Join(os.TempDir(), fileName+"_tmp.pb")
		cmd := exec.Command( //nolint:gosec
			"protoc",
			"--descriptor_set_out="+tmpFile,
			// include also files from the original dir + our proto file
			"-I"+absPath, filepath.Join(absPath, fileName))

		// redirect messages from the command
		// user should see an error if any
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			_ = os.Remove(tmpFile)
			return err
		}

		protoFile, err := os.ReadFile(tmpFile)
		if err != nil {
			_ = os.Remove(tmpFile)
			return err
		}

		fdSet := new(descriptorpb.FileDescriptorSet)
		err = proto.Unmarshal(protoFile, fdSet)
		if err != nil {
			_ = os.Remove(tmpFile)
			return err
		}

		// no files
		if len(fdSet.GetFile()) < 1 {
			continue
		}

		// we need only first
		pb := fdSet.GetFile()[0]

		fd, err := protodesc.NewFile(pb, protoregistry.GlobalFiles)
		if err != nil {
			_ = os.Remove(tmpFile)
			return err
		}

		// register file
		err = protoregistry.GlobalFiles.RegisterFile(fd)
		if err != nil {
			_ = os.Remove(tmpFile)
			return err
		}

		_ = os.Remove(tmpFile)
	}

	return nil
}
