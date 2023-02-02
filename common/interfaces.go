package common

import (
	"google.golang.org/grpc"
)

type UnaryInterceptor interface {
	Interceptor() grpc.UnaryServerInterceptor
	Name() string
}
