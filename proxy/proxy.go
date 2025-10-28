package proxy

import (
	"encoding/base64"
	"encoding/json"
	stderr "errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	_ "google.golang.org/genproto/protobuf/ptype" //nolint:revive,nolintlint

	"maps"

	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/goridge/v3/pkg/frame"
	"github.com/Sinersis/grpc/v5/codec"
	"github.com/roadrunner-server/pool/payload"
	"github.com/roadrunner-server/pool/pool/static_pool"
	"github.com/roadrunner-server/pool/worker"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	peerAddr     string = ":peer.address"
	peerAuthType string = ":peer.auth-type"
	delimiter    string = "|:|"
	apiErr       string = "error"
	headers      string = "headers"
	trailers     string = "trailers"
)

type Pool interface {
	// Workers return a worker list associated with the pool.
	Workers() (workers []*worker.Process)
	// Exec payload
	Exec(ctx context.Context, p *payload.Payload, stopCh chan struct{}) (chan *static_pool.PExec, error)
	// Reset kills all workers inside the watcher and replaces with new
	Reset(ctx context.Context) error
	// Destroy all underlying stacks (but let them complete the task).
	Destroy(ctx context.Context)
}

// base interface for Proxy class
type proxyService interface {
	// RegisterMethod registers a new RPC method.
	RegisterMethod(method string)

	// ServiceDesc returns a service description for the proxy.
	ServiceDesc() *grpc.ServiceDesc
}

// carry details about service, method and RPC context to a PHP process
type rpcContext struct {
	Service string              `json:"service"`
	Method  string              `json:"method"`
	Context map[string][]string `json:"context"`
}

// Proxy manages GRPC/RoadRunner bridge.
type Proxy struct {
	mu       *sync.RWMutex
	log      *zap.Logger
	prop     propagation.TextMapPropagator
	grpcPool Pool
	name     string
	metadata string
	methods  []string

	pldPool sync.Pool
}

// NewProxy creates a new service proxy object.
func NewProxy(name string, metadata string, log *zap.Logger, grpcPool Pool, mu *sync.RWMutex, prop propagation.TextMapPropagator) *Proxy {
	return &Proxy{
		log:      log,
		mu:       mu,
		prop:     prop,
		grpcPool: grpcPool,
		name:     name,
		metadata: metadata,
		methods:  make([]string, 0),
		pldPool: sync.Pool{
			New: func() any {
				return &payload.Payload{
					Codec:   frame.CodecJSON,
					Context: make([]byte, 0, 100),
					Body:    make([]byte, 0, 100),
				}
			},
		},
	}
}

// RegisterMethod registers a new RPC method.
func (p *Proxy) RegisterMethod(method string) {
	p.methods = append(p.methods, method)
}

// ServiceDesc returns a service description for the proxy.
func (p *Proxy) ServiceDesc() *grpc.ServiceDesc {
	desc := &grpc.ServiceDesc{
		ServiceName: p.name,
		Metadata:    p.metadata,
		HandlerType: (*proxyService)(nil),
		Methods:     []grpc.MethodDesc{},
		Streams:     []grpc.StreamDesc{},
	}

	// Registering methods
	for _, m := range p.methods {
		desc.Methods = append(desc.Methods, grpc.MethodDesc{
			MethodName: m,
			Handler:    p.methodHandler(m),
		})
	}

	return desc
}

// Generate method handler proxy.
// returns grpc method handler
/*
// MethodDesc represents an RPC service's method specification.
type MethodDesc struct {
	MethodName string
	Handler    methodHandler
}
*/
func (p *Proxy) methodHandler(method string) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		in := &codec.RawMessage{}
		if err := dec(in); err != nil {
			return nil, wrapError(err)
		}

		if interceptor == nil {
			return p.invoke(ctx, method, in)
		}

		info := &grpc.UnaryServerInfo{
			Server:     srv,
			FullMethod: fmt.Sprintf("/%s/%s", p.name, method),
		}

		handler := func(ctx context.Context, req any) (any, error) {
			switch r := req.(type) {
			case *codec.RawMessage:
				return p.invoke(ctx, method, r)
			default:
				return nil, errors.Errorf("unexpected request type %T", r)
			}
		}

		return interceptor(ctx, in, info, handler)
	}
}

func (p *Proxy) invoke(ctx context.Context, method string, in *codec.RawMessage) (any, error) {
	pld := p.getPld()
	defer p.putPld(pld)

	// experimental grpc API
	st := grpc.ServerTransportStreamFromContext(ctx)

	err := p.makePayload(ctx, method, in, pld)
	if err != nil {
		return nil, err
	}

	p.mu.RLock()
	re, err := p.grpcPool.Exec(ctx, pld, nil)
	p.mu.RUnlock()
	if err != nil {
		return nil, wrapError(err)
	}

	var r *payload.Payload

	select {
	case pl := <-re:
		if pl.Error() != nil {
			return nil, err
		}
		// streaming is not supported
		if pl.Payload().Flags&frame.STREAM != 0 {
			return nil, errors.Str("streaming is not supported")
		}

		// assign the payload
		r = pl.Payload()
	default:
		return nil, errors.Str("worker empty response")
	}

	err = p.responseMetadata(st, r)
	if err != nil {
		return nil, err
	}

	return codec.RawMessage(r.Body), nil
}

// responseMetadata extracts metadata from roadrunner response Payload.Context and converts it to metadata.MD
func (p *Proxy) responseMetadata(st grpc.ServerTransportStream, resp *payload.Payload) error {
	if resp == nil || len(resp.Context) == 0 {
		return nil
	}

	var rpcMetadata map[string]string
	err := json.Unmarshal(resp.Context, &rpcMetadata)
	if err != nil {
		return err
	}

	if len(rpcMetadata) > 0 {
		// old meta should not be used in response in new API
		md := metadata.New(rpcMetadata)

		// we assume that if there are no new headers and trailers, an old PHP-GRPC client is used
		if len(md.Get(headers)) == 0 && len(md.Get(trailers)) == 0 {
			// backward compatibility
			_ = st.SetHeader(md)
			goto parseErr
		}

		// New API headers
		if len(md.Get(headers)) > 0 {
			mdh := make(map[string]any)
			err = json.Unmarshal([]byte(md.Get(headers)[0]), &mdh)
			if err != nil {
				// we don't need to return this error, log it
				p.log.Error("error unmarshalling headers", zap.Error(err))
			}

			for k, v := range mdh {
				switch tt := v.(type) {
				case string:
					_ = st.SetHeader(metadata.Pairs(k, tt))
				case int:
					_ = st.SetHeader(metadata.Pairs(k, strconv.Itoa(tt)))
				default:
					p.log.Warn("skipping header with unsupported type", zap.String("key", k), zap.Any("value", v))
				}
			}
		}

		// New API trailers
		if len(md.Get(trailers)) > 0 {
			mdh := make(map[string]any)
			err = json.Unmarshal([]byte(md.Get(trailers)[0]), &mdh)
			if err != nil {
				// we don't need to return this error, log it
				p.log.Error("error unmarshalling trailers", zap.Error(err))
			}

			for k, v := range mdh {
				switch tt := v.(type) {
				case string:
					_ = st.SetTrailer(metadata.Pairs(k, tt))
				case int:
					_ = st.SetTrailer(metadata.Pairs(k, strconv.Itoa(tt)))
				default:
					p.log.Warn("skipping header with unsupported type", zap.String("key", k), zap.Any("value", v))
				}
			}
		}

		/*
			we have an error
			actually if the code is OK, status.ErrorProto will be nil,
			but we use this only in case of PHP exception happened
		*/
	parseErr:
		if len(md.Get(apiErr)) > 0 {
			sst := &spb.Status{}

			// get an error
			data, err := base64.StdEncoding.DecodeString(md.Get(apiErr)[0])
			if err != nil {
				return err
			}

			err = proto.Unmarshal(data, sst)
			if err != nil {
				return err
			}

			return status.ErrorProto(sst)
		}
	}

	return nil
}

// makePayload generates a RoadRunner compatible payload based on a GRPC message.
func (p *Proxy) makePayload(ctx context.Context, method string, body *codec.RawMessage, pld *payload.Payload) error {
	ctxMD := make(map[string][]string)

	p.prop.Inject(ctx, propagation.HeaderCarrier(ctxMD))
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		maps.Copy(ctxMD, md)
	}

	if pr, ok := peer.FromContext(ctx); ok {
		ctxMD[peerAddr] = []string{pr.Addr.String()}
		if pr.AuthInfo != nil {
			ctxMD[peerAuthType] = []string{pr.AuthInfo.AuthType()}
		}
	}

	ctxData, err := json.Marshal(rpcContext{Service: p.name, Method: method, Context: ctxMD})

	if err != nil {
		return err
	}

	pld.Body = *body
	pld.Context = ctxData

	return nil
}

func (p *Proxy) putPld(pld *payload.Payload) {
	pld.Body = nil
	pld.Context = nil
	p.pldPool.Put(pld)
}

func (p *Proxy) getPld() *payload.Payload {
	pld := p.pldPool.Get().(*payload.Payload)
	pld.Codec = frame.CodecJSON
	return pld
}

func GetOriginalErr(err error) string {
	var e *errors.Error
	ok := stderr.As(err, &e)
	if !ok {
		if err == nil {
			return ""
		}

		return err.Error()
	}

	if e != nil {
		return GetOriginalErr(e.Err)
	}

	return ""
}

// mounts proper error code for the error
func wrapError(err error) error {
	// internal agreement
	errMsg := GetOriginalErr(err)
	if strings.Contains(errMsg, delimiter) {
		chunks := strings.Split(errMsg, delimiter)
		code := codes.Internal

		// protect the slice access
		if len(chunks) < 2 {
			return err
		}

		phpCode, errConv := strconv.ParseUint(chunks[0], 10, 32)
		if errConv != nil {
			return err
		}

		if phpCode > 0 && phpCode < math.MaxUint32 {
			code = codes.Code(phpCode)
		}

		st := status.New(code, chunks[1]).Proto()

		for _, detailsMessage := range chunks[2:] {
			anyDetailsMessage := anypb.Any{}
			errP := proto.Unmarshal([]byte(detailsMessage), &anyDetailsMessage)
			if errP == nil {
				st.Details = append(st.Details, &anyDetailsMessage)
			}
		}

		return status.ErrorProto(st)
	}

	return status.Error(codes.Internal, err.Error())
}
