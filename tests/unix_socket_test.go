//go:build linux || darwin || freebsd

package grpc_test

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"tests/helpers"
	mocklogger "tests/mock"

	"github.com/roadrunner-server/config/v6"
	grpcPlugin "github.com/roadrunner-server/grpc/v6"
	"github.com/stretchr/testify/require"
)

func TestUnixSocketConfigRejectsInvalidOptions(t *testing.T) {
	log := mocklogger.NewLogger(slog.New(slog.DiscardHandler))

	for _, tc := range []struct {
		name    string
		listen  string
		options string
		wantErr string
	}{
		{name: "TCP options", listen: "tcp://127.0.0.1:0", options: `{mode: "0600"}`, wantErr: "filesystem unix:// address"},
		{name: "invalid mode", listen: "unix://grpc.sock", options: `{mode: "0780"}`, wantErr: "invalid unix socket mode"},
		{name: "unquoted mode", listen: "unix://grpc.sock", options: "{mode: 0600}", wantErr: "invalid unix socket mode"},
		{name: "empty socket path", listen: "unix://", options: `{mode: "0600"}`, wantErr: "filesystem unix:// address"},
		{name: "scalar options", listen: "unix://grpc.sock", options: "false", wantErr: "expected a map"},
		{name: "negative UID", listen: "unix://grpc.sock", options: "{uid: -1}", wantErr: "invalid unix socket uid"},
		{name: "negative GID", listen: "unix://grpc.sock", options: "{gid: -1}", wantErr: "invalid unix socket gid"},
		{name: "reserved UID", listen: "unix://grpc.sock", options: "{uid: 4294967295}", wantErr: "invalid unix socket uid"},
		{name: "reserved GID", listen: "unix://grpc.sock", options: "{gid: 4294967295}", wantErr: "invalid unix socket gid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Plugin{Path: unixSocketConfig(t, tc.listen, tc.options)}
			require.NoError(t, cfg.Init())
			p := &grpcPlugin.Plugin{}
			require.ErrorContains(t, p.Init(cfg, log, nil), tc.wantErr)
		})
	}
}

func TestUnixSocketMode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fileMode string
		flags    []string
		wantMode os.FileMode
	}{
		{name: "quoted 0600", fileMode: "0600", wantMode: 0o600},
		{name: "quoted 0640", fileMode: "0640", wantMode: 0o640},
		{name: "mode override", fileMode: "0600", flags: []string{"grpc.unix_socket.mode=0640"}, wantMode: 0o640},
	} {
		t.Run(tc.name, func(t *testing.T) {
			socket := unixSocketPath(t)
			cfgPath := unixSocketConfig(t, "unix://"+socket, fmt.Sprintf(`{mode: %q}`, tc.fileMode))
			helpers.Start(t, cfgPath, grpcPlugins(), helpers.WithConfigFlags(tc.flags...))

			info, err := os.Stat(socket)
			require.NoError(t, err)
			require.Equal(t, tc.wantMode, info.Mode().Perm())
		})
	}
}

func TestUnixSocketPing(t *testing.T) {
	socket := unixSocketPath(t)
	cfgPath := unixSocketConfig(t, "unix://"+socket, `{mode: "0600"}`)
	helpers.Start(t, cfgPath, grpcPlugins())

	got, err := ping(t, helpers.Dial(t, "unix://"+socket), "TOST")

	require.NoError(t, err)
	require.Equal(t, "TOST", got)
}

func TestUnixSocketStopRemovesListener(t *testing.T) {
	socket := unixSocketPath(t)
	cfgPath := unixSocketConfig(t, "unix://"+socket, `{mode: "0600"}`)
	_, stop := helpers.Start(t, cfgPath, grpcPlugins())
	require.FileExists(t, socket)

	stop()

	require.NoFileExists(t, socket)
}

func TestUnixSocketOwnershipErrorRemovesListener(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Requires an unprivileged process.")
	}

	socket := unixSocketPath(t)
	cfgPath := unixSocketConfig(t, "unix://"+socket, "{uid: 0}")
	err := helpers.StartExpectServeError(t, cfgPath, grpcPlugins())
	require.ErrorContains(t, err, "chown unix socket")

	_, err = os.Stat(socket)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func unixSocketPath(t *testing.T) string {
	t.Helper()

	// Keep the socket path below the macOS length limit.
	dir, err := os.MkdirTemp("", "rr-grpc-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	return filepath.Join(dir, "grpc.sock")
}

func unixSocketConfig(t *testing.T, listen, options string) string {
	t.Helper()

	contents := fmt.Sprintf(`version: "3"
server:
  command: "php php_test_files/worker-grpc.php"
grpc:
  listen: %q
  proto:
    - "proto/service/service.proto"
  unix_socket: %s
  pool:
    debug: true
    destroy_timeout: 5s
`, listen, options)
	path := filepath.Join(t.TempDir(), ".rr.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}
