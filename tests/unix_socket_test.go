//go:build linux || darwin || freebsd

package grpc_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"tests/helpers"
	mocklogger "tests/mock"

	"github.com/roadrunner-server/config/v6"
	grpcPlugin "github.com/roadrunner-server/grpc/v6"
	"github.com/roadrunner-server/server/v6"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
)

func TestUnixSocketConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	log := mocklogger.NewLogger(slog.New(slog.DiscardHandler))

	for _, tc := range []struct {
		name    string
		listen  string
		options string
		flags   []string
		mode    string
		zeroIDs bool
		wantErr string
	}{
		{name: "omitted", listen: "tcp://127.0.0.1:0"},
		{name: "null", listen: "tcp://127.0.0.1:0", options: "null"},
		{name: "quoted mode", listen: "unix://grpc.sock", options: `{mode: "0000"}`, mode: "0000"},
		{name: "zero IDs", listen: "unix://grpc.sock", options: "{uid: 0, gid: 0}", zeroIDs: true},
		{name: "string overrides", listen: "unix://grpc.sock", options: `{mode: "0600"}`, flags: []string{"grpc.unix_socket.mode=0640", "grpc.unix_socket.uid=0", "grpc.unix_socket.gid=0"}, mode: "0640", zeroIDs: true},
		{name: "TCP options", listen: "tcp://127.0.0.1:0", options: `{mode: "0600"}`, wantErr: "grpc.unix_socket"},
		{name: "empty TCP options", listen: "tcp://127.0.0.1:0", options: "{}", wantErr: "grpc.unix_socket"},
		{name: "invalid mode", listen: "unix://grpc.sock", options: `{mode: "0780"}`, wantErr: "grpc.unix_socket"},
		{name: "unquoted mode", listen: "unix://grpc.sock", options: "{mode: 0600}", wantErr: "grpc.unix_socket"},
		{name: "empty address", options: "{}", wantErr: "malformed grpc address"},
		{name: "empty socket address", listen: "unix://", options: "{}", wantErr: "grpc.unix_socket"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := unixSocketConfig(t, tc.listen, tc.options, tc.flags)
			var decoded grpcPlugin.Config
			require.NoError(t, cfg.UnmarshalKey("grpc", &decoded))

			p := &grpcPlugin.Plugin{}
			err := p.Init(cfg, log, nil)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.NoFileExists(t, "grpc.sock")
				return
			}
			require.NoError(t, err)
			require.NoError(t, decoded.InitDefaults())
			require.Equal(t, tc.listen, decoded.Listen)
			if tc.options == "" || tc.options == "null" {
				require.Nil(t, decoded.UnixSocket)
				return
			}
			require.NotNil(t, decoded.UnixSocket)
			require.Equal(t, tc.mode, decoded.UnixSocket.Mode)
			if tc.zeroIDs {
				require.NotNil(t, decoded.UnixSocket.UID)
				require.NotNil(t, decoded.UnixSocket.GID)
				require.Zero(t, *decoded.UnixSocket.UID)
				require.Zero(t, *decoded.UnixSocket.GID)
			} else {
				require.Nil(t, decoded.UnixSocket.UID)
				require.Nil(t, decoded.UnixSocket.GID)
			}
		})
	}
}

func TestUnixSocketIDs(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("RR_TEST_UNIX_SOCKET_ID", "33")
	t.Setenv("RR_TEST_UNIX_SOCKET_UNSET_ID", "")
	require.NoError(t, os.Unsetenv("RR_TEST_UNIX_SOCKET_UNSET_ID"))
	log := mocklogger.NewLogger(slog.New(slog.DiscardHandler))

	for _, field := range []string{"uid", "gid"} {
		for _, tc := range []struct {
			name        string
			value       string
			valid       bool
			want        int
			json        bool
			parentError bool
		}{
			{name: "false", value: "false"},
			{name: "true", value: "true"},
			{name: "fraction", value: "1.9"},
			{name: "negative fraction", value: "-0.5"},
			{name: "empty string", value: `""`},
			{name: "unset environment", value: `"${RR_TEST_UNIX_SOCKET_UNSET_ID}"`},
			{name: "negative ID", value: "-1"},
			{name: "oversized ID", value: "4294967295"},
			{name: "oversized float", value: "4294967295.0"},
			{name: "oversized unsigned ID", value: "18446744073709551615"},
			{name: "oversized string ID", value: `"4294967295"`},
			{name: "string overflow", value: `"18446744073709551616"`, parentError: true},
			{name: "NaN", value: ".nan"},
			{name: "infinity", value: ".inf"},
			{name: "map", value: "{id: 33}", parentError: true},
			{name: "slice", value: "[33]", parentError: true},
			{name: "JSON fraction", value: "1.9", json: true},
			{name: "zero", value: "0", valid: true},
			{name: "null", value: "null", valid: true},
			{name: "integer", value: "33", valid: true, want: 33},
			{name: "integer float", value: "33.0", valid: true, want: 33},
			{name: "JSON integer float", value: "33.0", valid: true, want: 33, json: true},
			{name: "environment", value: `"${RR_TEST_UNIX_SOCKET_ID}"`, valid: true, want: 33},
			{name: "octal string", value: `"041"`, valid: true, want: 33},
			{name: "hexadecimal string", value: `"0x21"`, valid: true, want: 33},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				var cfg *config.Plugin
				if tc.json {
					path := filepath.Join(t.TempDir(), ".rr.json")
					contents := fmt.Sprintf(`{"version":"3","grpc":{"listen":"unix://grpc.sock","unix_socket":{"mode":"0600",%q:%s}}}`, field, tc.value)
					require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
					cfg = &config.Plugin{Path: path}
					require.NoError(t, cfg.Init())
				} else {
					options := fmt.Sprintf(`{mode: "0600", %s: %s}`, field, tc.value)
					cfg = unixSocketConfig(t, "unix://grpc.sock", options, nil)
				}
				p := &grpcPlugin.Plugin{}
				err := p.Init(cfg, log, nil)
				require.NoFileExists(t, "grpc.sock")
				if !tc.valid {
					prefix := "grpc.unix_socket."
					if tc.parentError {
						prefix = "unix_socket."
					}
					require.ErrorContains(t, err, prefix+field)
					return
				}
				require.NoError(t, err)
				var decoded grpcPlugin.Config
				require.NoError(t, cfg.UnmarshalKey("grpc", &decoded))
				require.NotNil(t, decoded.UnixSocket)
				id, other := decoded.UnixSocket.UID, decoded.UnixSocket.GID
				if field == "gid" {
					id, other = other, id
				}
				require.Nil(t, other)
				if tc.value == "null" {
					require.Nil(t, id)
				} else {
					require.NotNil(t, id)
					require.Equal(t, tc.want, *id)
				}
			})
		}
	}
}

func TestUnixSocketServe(t *testing.T) {
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
			options := fmt.Sprintf(`{mode: "0600", uid: %d, gid: %d}`, os.Getuid(), os.Getgid())
			if tc.mode == 0 {
				var lc net.ListenConfig
				ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
				require.NoError(t, err)
				target = ln.Addr().String()
				listen, options = "tcp://"+target, ""
				require.NoError(t, ln.Close())
			}
			cfg := unixSocketConfig(t, listen, options, tc.flags)
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
			errCh := p.Serve()
			select {
			case err := <-errCh:
				t.Fatalf("gRPC serve: %v", err)
			default:
			}
			require.Empty(t, p.Workers())

			if tc.mode != 0 {
				info, err := os.Stat("grpc.sock")
				require.NoError(t, err)
				require.Equal(t, tc.mode, info.Mode().Perm())
				stat, ok := info.Sys().(*syscall.Stat_t)
				require.True(t, ok)
				require.EqualValues(t, os.Getuid(), stat.Uid)
				require.EqualValues(t, os.Getgid(), stat.Gid)
			}

			conn := helpers.Dial(t, target)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			health, err := grpchealth.NewHealthClient(conn).Check(ctx, &grpchealth.HealthCheckRequest{}, grpc.WaitForReady(true))
			require.NoError(t, err)
			require.Equal(t, grpchealth.HealthCheckResponse_SERVING, health.GetStatus())
		})
	}
}

func unixSocketConfig(t *testing.T, listen, options string, flags []string) *config.Plugin {
	t.Helper()
	contents := fmt.Sprintf("version: \"3\"\nserver:\n  command: [unused]\ngrpc:\n  listen: %q\n  pool:\n    debug: true\n", listen)
	if options != "" {
		contents += "  unix_socket: " + options + "\n"
	}
	path := filepath.Join(t.TempDir(), ".rr.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	cfg := &config.Plugin{Path: path, Flags: flags}
	require.NoError(t, cfg.Init())
	return cfg
}
