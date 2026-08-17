package grpc_test

import (
	"testing"

	"tests/helpers"

	"github.com/roadrunner-server/resetter/v6"
	"github.com/stretchr/testify/require"
)

// The gzip cases mirror the plain ones with a compressing connection. They used
// to be a separate 452-line file repeating every scenario; the only difference
// is the dial option, so they reuse the same configs and helpers here.

func TestPingEchoesMessageGzip(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq.yaml", grpcPlugins(), helpers.WithTCPProbe(rqAddr))

	got, err := ping(t, helpers.Dial(t, rqAddr, helpers.Gzip()), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

func TestTLSGzip(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-tls.yaml", grpcPlugins(), helpers.WithTCPProbe(tlsAddr))

	got, err := ping(t, helpers.DialTLS(t, tlsAddr, helpers.Gzip()), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

func TestMutualTLSGzip(t *testing.T) {
	helpers.Start(t, "configs/.rr-grpc-rq-tls-rootca.yaml", grpcPlugins(), helpers.WithTCPProbe(tlsRootCAAddr))

	got, err := ping(t, helpers.DialMutualTLS(t, tlsRootCAAddr, helpers.Gzip()), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

func TestTLSGzipSurvivesReset(t *testing.T) {
	helpers.Start(t,
		"configs/.rr-grpc-rq-tls.yaml",
		append(grpcPlugins(), &resetter.Plugin{}),
		helpers.WithTCPProbe(tlsAddr),
	)

	conn := helpers.DialTLS(t, tlsAddr, helpers.Gzip())

	got, err := ping(t, conn, "BEFORE")
	require.NoError(t, err)
	require.Equal(t, "BEFORE", got)

	resetAll(t, tlsRPCAddr)

	got, err = ping(t, conn, "AFTER")
	require.NoError(t, err)
	require.Equal(t, "AFTER", got)
}
