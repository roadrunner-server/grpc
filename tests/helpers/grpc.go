package helpers

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
)

const (
	// clientCert and clientKey are presented when the server asks for a client
	// certificate (client_auth_type: require_and_verify_client_cert).
	clientCert = "test-certs/localhost+2-client.pem"
	clientKey  = "test-certs/localhost+2-client-key.pem"
	// rootCA signed all of the above; CI creates them with mkcert, see linux.yml.
	rootCA = "test-certs/rootCA.pem"
)

// Dial opens a plaintext connection and closes it on cleanup.
func Dial(t *testing.T, addr string, opts ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()

	conn, err := grpc.NewClient(addr, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// DialTLS opens a connection that trusts the test CA but sends no client
// certificate.
func DialTLS(t *testing.T, addr string, opts ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()

	conn, err := grpc.NewClient(addr, append([]grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		RootCAs:    testRootCAs(t),
		MinVersion: tls.VersionTLS12,
	}))}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// DialMutualTLS opens a connection presenting the client certificate, which the
// server requires when client_auth_type is set.
func DialMutualTLS(t *testing.T, addr string, opts ...grpc.DialOption) *grpc.ClientConn {
	t.Helper()

	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	require.NoError(t, err)

	conn, err := grpc.NewClient(addr, append([]grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      testRootCAs(t),
		MinVersion:   tls.VersionTLS12,
	}))}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// testRootCAs returns a pool holding the CA that signed the test certificates.
func testRootCAs(t *testing.T) *x509.CertPool {
	t.Helper()

	pem, err := os.ReadFile(rootCA)
	require.NoError(t, err, "generate the certificates with mkcert first, see .github/workflows/linux.yml")

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(pem), "%s holds no usable certificate", rootCA)

	return pool
}

// DialRPC opens a plain tcp connection to the goridge rpc listener.
func DialRPC(t *testing.T, addr string) net.Conn {
	t.Helper()

	conn, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// Gzip makes the connection compress request and response bodies.
func Gzip() grpc.DialOption {
	return grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name))
}
