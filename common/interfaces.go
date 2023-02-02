package common

import (
	"google.golang.org/grpc"
)

type Middleware interface {
	Middleware() grpc.UnaryServerInterceptor
	Name() string
}