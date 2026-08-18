package grpc_test

import (
	"context"
	"testing"
	"time"

	"tests/helpers"

	"github.com/roadrunner-server/protoreg/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpcreflectv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

const reflectionAddr = "127.0.0.1:9099"

// TestReflectionListsService drives the server-reflection API against a running
// server.
func TestReflectionListsService(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-grpc-reflection.yaml",
		append(grpcPlugins(), &protoreg.Plugin{}),
		helpers.WithTCPProbe(reflectionAddr),
	)

	assertReflection(t, helpers.Dial(t, reflectionAddr))
}

// assertReflection drives the v1 server-reflection API: it lists the registered
// services, then resolves the file descriptor that contains the proxied
// service. A non-empty descriptor proves the protoreg registry is actually
// backing reflection (plain reflection over the dynamic proxy could not serve
// the file descriptors).
func assertReflection(t *testing.T, conn *grpc.ClientConn) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	client := grpcreflectv1.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	require.NoError(t, err)

	err = stream.Send(&grpcreflectv1.ServerReflectionRequest{
		MessageRequest: &grpcreflectv1.ServerReflectionRequest_ListServices{ListServices: "*"},
	})
	require.NoError(t, err)

	resp, err := stream.Recv()
	require.NoError(t, err)

	services := resp.GetListServicesResponse().GetService()
	names := make([]string, 0, len(services))
	for _, svc := range services {
		names = append(names, svc.GetName())
	}
	require.Contains(t, names, "service.Echo")

	err = stream.Send(&grpcreflectv1.ServerReflectionRequest{
		MessageRequest: &grpcreflectv1.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: "service.Echo"},
	})
	require.NoError(t, err)

	resp, err = stream.Recv()
	require.NoError(t, err)

	fdResp := resp.GetFileDescriptorResponse()
	require.NotNil(t, fdResp)
	require.NotEmpty(t, fdResp.GetFileDescriptorProto())
}
