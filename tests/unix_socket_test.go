//go:build linux || darwin || freebsd

package grpc_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"tests/helpers"
	mocklogger "tests/mock"
	"tests/proto/service"

	"github.com/roadrunner-server/config/v6"
	grpcPlugin "github.com/roadrunner-server/grpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

func TestUnixSocketConfig(t *testing.T) {
	log := mocklogger.NewLogger(slog.New(slog.DiscardHandler))

	for _, tc := range []struct {
		name    string
		listen  string
		options string
		wantErr string
	}{
		{name: "TCP defaults", listen: "tcp://127.0.0.1:0"},
		{name: "UNIX defaults", listen: "unix://grpc.sock"},
		{name: "empty options", listen: "unix://grpc.sock", options: "{}"},
		{name: "TCP empty options", listen: "tcp://127.0.0.1:0", options: "{}"},
		{name: "mode only", listen: "unix://grpc.sock", options: `{mode: "0600"}`},
		{name: "explicit zero", listen: "unix://grpc.sock", options: `{mode: "0000", uid: 0, gid: 0}`},
		{name: "unset mode", listen: "unix://grpc.sock", options: "{uid: 0, gid: 0}"},
		{name: "TCP options", listen: "tcp://127.0.0.1:0", options: `{mode: "0600"}`, wantErr: "filesystem unix:// address"},
		{name: "invalid mode", listen: "unix://grpc.sock", options: `{mode: "0780"}`, wantErr: "invalid unix socket mode"},
		{name: "unquoted mode", listen: "unix://grpc.sock", options: "{mode: 0600}", wantErr: "invalid unix socket mode"},
		{name: "empty address", options: `{mode: "0600"}`, wantErr: "malformed grpc address"},
		{name: "empty socket path", listen: "unix://", options: `{mode: "0600"}`, wantErr: "filesystem unix:// address"},
		{name: "scalar options", listen: "unix://grpc.sock", options: "false", wantErr: "expected a map"},
		{name: "negative UID", listen: "unix://grpc.sock", options: "{uid: -1}", wantErr: "invalid unix socket uid"},
		{name: "negative GID", listen: "unix://grpc.sock", options: "{gid: -1}", wantErr: "invalid unix socket gid"},
		{name: "reserved UID", listen: "unix://grpc.sock", options: "{uid: 4294967295}", wantErr: "invalid unix socket uid"},
		{name: "reserved GID", listen: "unix://grpc.sock", options: "{gid: 4294967295}", wantErr: "invalid unix socket gid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := unixSocketConfig(t, tc.listen, tc.options, nil)
			p := &grpcPlugin.Plugin{}
			err := p.Init(cfg, log, nil)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestUnixSocketServe(t *testing.T) {
	worker, err := filepath.Abs("php_test_files/worker-grpc.php")
	require.NoError(t, err)
	proto, err := filepath.Abs("proto/service/service.proto")
	require.NoError(t, err)
	t.Setenv("RR_TEST_SOCKET_UID", strconv.Itoa(os.Getuid()))
	t.Setenv("RR_TEST_SOCKET_GID", strconv.Itoa(os.Getgid()))

	for _, tc := range []struct {
		name  string
		flags []string
		mode  os.FileMode
	}{
		{name: "TCP without options"},
		{name: "quoted mode", mode: 0o600},
		{name: "string override", flags: []string{"grpc.unix_socket.mode=0640"}, mode: 0o640},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			listen, target := "unix://grpc.sock", "unix:grpc.sock"
			options := `{mode: "0600", uid: "${RR_TEST_SOCKET_UID}", gid: "${RR_TEST_SOCKET_GID}"}`
			if tc.mode == 0 {
				var lc net.ListenConfig
				ln, errL := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
				require.NoError(t, errL)
				target = ln.Addr().String()
				listen, options = "tcp://"+target, ""
				require.NoError(t, ln.Close())
			}
			flags := append([]string{"server.command=php " + worker, "grpc.proto=" + proto}, tc.flags...)
			cfg := unixSocketConfig(t, listen, options, flags)
			log := mocklogger.NewLogger(slog.New(slog.DiscardHandler))
			rrServer := &server.Plugin{}
			require.NoError(t, rrServer.Init(cfg, log))
			t.Cleanup(func() { require.NoError(t, rrServer.Stop(context.Background())) })
			p := &grpcPlugin.Plugin{}
			require.NoError(t, p.Init(cfg, log, rrServer))
			stop := sync.OnceValue(func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return p.Stop(ctx)
			})
			t.Cleanup(func() { require.NoError(t, stop()) })
			errCh := p.Serve()
			select {
			case errS := <-errCh:
				t.Fatalf("gRPC serve: %v", errS)
			default:
			}

			conn := helpers.Dial(t, target)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			response, errR := service.NewEchoClient(conn).Ping(ctx, &service.Message{Msg: tc.name}, grpc.WaitForReady(true))
			require.NoError(t, errR)
			require.Equal(t, strings.ToUpper(tc.name), response.GetMsg())
			health, errH := grpchealth.NewHealthClient(conn).Check(ctx, &grpchealth.HealthCheckRequest{})
			require.NoError(t, errH)
			require.Equal(t, grpchealth.HealthCheckResponse_SERVING, health.GetStatus())

			if tc.mode != 0 {
				info, errS := os.Stat("grpc.sock")
				require.NoError(t, errS)
				require.NotZero(t, info.Mode()&os.ModeSocket)
				require.Equal(t, tc.mode, info.Mode().Perm())
				stat := info.Sys().(*syscall.Stat_t)
				require.EqualValues(t, os.Getuid(), stat.Uid)
				require.EqualValues(t, os.Getgid(), stat.Gid)
			}
			require.NoError(t, conn.Close())
			require.NoError(t, stop())
			if tc.mode != 0 {
				_, errS := os.Stat("grpc.sock")
				require.ErrorIs(t, errS, os.ErrNotExist)
			}
		})
	}
}

func TestUnixSocketOwnershipError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Requires an unprivileged process.")
	}
	groups, err := os.Getgroups()
	require.NoError(t, err)
	otherGID := 0
	for otherGID == os.Getegid() || slices.Contains(groups, otherGID) {
		otherGID++
	}

	for _, tc := range []struct {
		field string
		id    int
	}{
		{field: "uid", id: 0},
		{field: "gid", id: otherGID},
	} {
		t.Run(tc.field, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("RR_TEST_SOCKET_ID", strconv.Itoa(tc.id))
			options := fmt.Sprintf(`{%s: "${RR_TEST_SOCKET_ID}"}`, tc.field)
			cfg := unixSocketConfig(t, "unix://ownership.sock", options, nil)
			log := mocklogger.NewLogger(slog.New(slog.DiscardHandler))
			rrServer := &server.Plugin{}
			require.NoError(t, rrServer.Init(cfg, log))
			t.Cleanup(func() { require.NoError(t, rrServer.Stop(context.Background())) })
			p := &grpcPlugin.Plugin{}
			require.NoError(t, p.Init(cfg, log, rrServer))
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, p.Stop(ctx))
			})
			select {
			case errS := <-p.Serve():
				require.ErrorContains(t, errS, "chown unix socket")
			case <-time.After(5 * time.Second):
				t.Fatal("gRPC did not report the ownership error")
			}
			_, errS := os.Stat("ownership.sock")
			require.ErrorIs(t, errS, os.ErrNotExist)
		})
	}
}

func unixSocketConfig(t *testing.T, listen, options string, flags []string) *config.Plugin {
	t.Helper()
	contents := fmt.Sprintf(`version: "3"
server:
  command: [unused]
grpc:
  listen: %q
  pool:
    debug: true
    destroy_timeout: 5s
`, listen)
	if options != "" {
		contents += "  unix_socket: " + options + "\n"
	}
	path := filepath.Join(t.TempDir(), ".rr.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	cfg := &config.Plugin{Path: path, Flags: flags}
	require.NoError(t, cfg.Init())
	return cfg
}
