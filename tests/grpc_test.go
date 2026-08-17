package grpc_test

import (
	"testing"

	"tests/helpers"
	"tests/proto/service"

	grpcPlugin "github.com/roadrunner-server/grpc/v6"
	rpcPlugin "github.com/roadrunner-server/rpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	initAddr      = "127.0.0.1:9091"
	rqAddr        = "127.0.0.1:9001"
	multipleAddr  = "127.0.0.1:9003"
	exceptionAddr = "127.0.0.1:9001"
	issue1193Addr = "127.0.0.1:9001"
)

func grpcPlugins() []any {
	return []any{&grpcPlugin.Plugin{}, &rpcPlugin.Plugin{}, &server.Plugin{}}
}

// ping sends one Ping through the connection and returns the echoed message.
func ping(t *testing.T, conn *grpc.ClientConn, msg string) (string, error) {
	t.Helper()

	resp, err := service.NewEchoClient(conn).Ping(t.Context(), &service.Message{Msg: msg})
	if err != nil {
		return "", err
	}

	return resp.GetMsg(), nil
}

// TestBoots covers the plain init config: the server comes up and serves.
func TestBoots(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-init.yaml", grpcPlugins(), helpers.WithTCPProbe(initAddr))
}

// TestPingEchoesMessage is the basic request/response path over plaintext.
func TestPingEchoesMessage(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq.yaml", grpcPlugins(), helpers.WithTCPProbe(rqAddr))

	got, err := ping(t, helpers.Dial(t, rqAddr), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

// TestMultipleProtoFiles covers a config listing more than one proto file. The
// echo service still answers, and the health service the plugin registers
// alongside it reports serving, for the whole server and for the named service.
func TestMultipleProtoFiles(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-multiple.yaml", grpcPlugins(), helpers.WithTCPProbe(multipleAddr))

	conn := helpers.Dial(t, multipleAddr)

	got, err := ping(t, conn, "TOST")
	require.NoError(t, err)
	require.Equal(t, "TOST", got)

	health := grpchealth.NewHealthClient(conn)

	server, err := health.Check(t.Context(), &grpchealth.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpchealth.HealthCheckResponse_SERVING, server.GetStatus())

	svc, err := health.Check(t.Context(), &grpchealth.HealthCheckRequest{Service: "service.Echo"})
	require.NoError(t, err)
	require.Equal(t, grpchealth.HealthCheckResponse_SERVING, svc.GetStatus())
}

// TestWorkerExceptionIsReported checks a throwing worker surfaces as an rpc
// error rather than a hang or an empty success.
func TestWorkerExceptionIsReported(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-exception.yaml", grpcPlugins(), helpers.WithTCPProbe(exceptionAddr))

	_, err := ping(t, helpers.Dial(t, exceptionAddr), "TOST")

	require.Error(t, err)
}

// TestStdoutGarbageIsReportedInFull covers issue 1193. The worker prints to
// stdout, which corrupts the goridge frame, and the resulting error has to
// carry the whole offending message rather than a truncated prefix.
func TestStdoutGarbageIsReportedInFull(t *testing.T) {
	err := helpers.StartExpectServeError(t, "configs/.rr-grpc-rq-issue1193.yaml", grpcPlugins())

	require.ErrorContains(t, err, "If you want to be c001 you just need to contribute to 0pensource.")
}
