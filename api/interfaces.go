package api

import (
	"context"

	"github.com/jhump/protoreflect/v2/protoresolve"
	"github.com/roadrunner-server/pool/payload"
	"github.com/roadrunner-server/pool/pool"
	staticPool "github.com/roadrunner-server/pool/pool/static_pool"
	"github.com/roadrunner-server/pool/worker"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Registry interface {
	Registry() *protoresolve.Registry
	Services() map[string]protoreflect.ServiceDescriptor
	FindMethodByFullPath(method string) (protoreflect.MethodDescriptor, error)
}

type Interceptor interface {
	UnaryServerInterceptor() grpc.UnaryServerInterceptor
	Name() string
}

type Configurer interface {
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error
	// Has checks if a config section exists.
	Has(name string) bool
	// Experimental returns true if experimental mode is enabled.
	Experimental() bool
}

type Pool interface {
	// Workers return a worker list associated with the pool.
	Workers() (workers []*worker.Process)
	// Exec payload
	Exec(ctx context.Context, p *payload.Payload, stopCh chan struct{}) (chan *staticPool.PExec, error)
	// RemoveWorker removes worker from the pool.
	RemoveWorker(ctx context.Context) error
	// AddWorker adds worker to the pool.
	AddWorker() error
	// Reset kill all workers inside the watcher and replaces with new
	Reset(ctx context.Context) error
	// Destroy the underlying stack (but let them complete the task).
	Destroy(ctx context.Context)
	// QueueSize shows the number of requests in the queue
	QueueSize() uint64
}

type Logger interface {
	NamedLogger(name string) *zap.Logger
}

// Server creates workers for the application.
type Server interface {
	NewPool(ctx context.Context, cfg *pool.Config, env map[string]string, _ *zap.Logger) (*staticPool.Pool, error)
}
