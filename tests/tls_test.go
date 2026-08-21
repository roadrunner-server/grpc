package grpc_test

import (
	"net/rpc"
	"testing"

	"tests/helpers"

	goridgeRpc "github.com/roadrunner-server/goridge/v4/pkg/rpc"
	"github.com/roadrunner-server/resetter/v6"
	"github.com/stretchr/testify/require"
)

const (
	tlsAddr       = "127.0.0.1:9002"
	tlsRootCAAddr = "127.0.0.1:9003"
	tlsRPCAddr    = "127.0.0.1:6009"
	rootCARPCAddr = "127.0.0.1:6001"
)

// TestTLS serves with a certificate and no client auth, so trusting the CA is
// enough to call.
func TestTLS(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-tls.yaml", grpcPlugins(), helpers.WithTCPProbe(tlsAddr))

	got, err := ping(t, helpers.DialTLS(t, tlsAddr), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

// TestMutualTLS uses client_auth_type require_and_verify_client_cert, so the
// call only succeeds when the client presents its certificate.
func TestMutualTLS(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-tls-rootca.yaml", grpcPlugins(), helpers.WithTCPProbe(tlsRootCAAddr))

	got, err := ping(t, helpers.DialMutualTLS(t, tlsRootCAAddr), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

// TestMutualTLSRejectsClientWithoutCertificate is the negative half the suite
// was missing: without a client certificate the server must refuse the call.
func TestMutualTLSRejectsClientWithoutCertificate(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-tls-rootca.yaml", grpcPlugins(), helpers.WithTCPProbe(tlsRootCAAddr))

	_, err := ping(t, helpers.DialTLS(t, tlsRootCAAddr), "TOST")

	require.Error(t, err, "the server accepted a client that presented no certificate")
}

// TestTLSSurvivesReset resets the worker pool over rpc and checks the server is
// still usable afterwards, which is where a botched pool swap would show up.
func TestTLSSurvivesReset(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-grpc-rq-tls.yaml",
		append(grpcPlugins(), &resetter.Plugin{}),
		helpers.WithTCPProbe(tlsAddr),
	)

	conn := helpers.DialTLS(t, tlsAddr)

	got, err := ping(t, conn, "BEFORE")
	require.NoError(t, err)
	require.Equal(t, "BEFORE", got)

	resetAll(t, tlsRPCAddr)

	got, err = ping(t, conn, "AFTER")
	require.NoError(t, err)
	require.Equal(t, "AFTER", got)
}

// TestMutualTLSSurvivesReset is the same over the mutually authenticated port.
func TestMutualTLSSurvivesReset(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-grpc-rq-tls-rootca.yaml",
		append(grpcPlugins(), &resetter.Plugin{}),
		helpers.WithTCPProbe(tlsRootCAAddr),
	)

	conn := helpers.DialMutualTLS(t, tlsRootCAAddr)

	got, err := ping(t, conn, "BEFORE")
	require.NoError(t, err)
	require.Equal(t, "BEFORE", got)

	resetAll(t, rootCARPCAddr)

	got, err = ping(t, conn, "AFTER")
	require.NoError(t, err)
	require.Equal(t, "AFTER", got)
}

// resetAll asks the resetter plugin to rebuild every pool.
func resetAll(t *testing.T, rpcAddr string) {
	t.Helper()

	conn := helpers.DialRPC(t, rpcAddr)
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	t.Cleanup(func() { _ = client.Close() })

	var plugins []string
	require.NoError(t, client.Call("resetter.List", true, &plugins))
	require.Contains(t, plugins, "grpc")

	var done bool
	require.NoError(t, client.Call("resetter.Reset", "grpc", &done))
	require.True(t, done)
}
