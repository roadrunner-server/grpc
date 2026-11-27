package grpc

import (
	"context"
	"net/http"
	"os/exec"
	"sync"
	"testing"

	"github.com/roadrunner-server/grpc/v6/api"
	"github.com/roadrunner-server/pool/v2/fsm"
	"github.com/roadrunner-server/pool/v2/payload"
	staticPool "github.com/roadrunner-server/pool/v2/pool/static_pool"
	"github.com/roadrunner-server/pool/v2/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStatusPool is an api.Pool whose only meaningful method is Workers; the
// remaining methods exist solely to satisfy the interface so Plugin.Status and
// Plugin.Ready can be exercised without a running worker pool.
type fakeStatusPool struct {
	workers []*worker.Process
}

func (f *fakeStatusPool) Workers() []*worker.Process { return f.workers }
func (f *fakeStatusPool) Exec(context.Context, *payload.Payload, chan struct{}) (chan *staticPool.PExec, error) {
	return nil, nil
}
func (f *fakeStatusPool) RemoveWorker(context.Context) error { return nil }
func (f *fakeStatusPool) AddWorker() error                   { return nil }
func (f *fakeStatusPool) Reset(context.Context) error        { return nil }
func (f *fakeStatusPool) Destroy(context.Context)            {}
func (f *fakeStatusPool) QueueSize() uint64                  { return 0 }

// newWorker builds an unstarted worker driven into the requested FSM state.
func newWorker(t *testing.T, state int64) *worker.Process {
	t.Helper()
	w, err := worker.InitBaseWorker(exec.CommandContext(t.Context(), "grpc-status-test-worker"))
	require.NoError(t, err)

	switch state {
	case fsm.StateReady:
		w.State().Transition(fsm.StateReady)
	case fsm.StateWorking:
		w.State().Transition(fsm.StateReady)
		w.State().Transition(fsm.StateWorking)
	}
	return w
}

func TestPluginStatusAndReady(t *testing.T) {
	tests := []struct {
		name       string
		pool       api.Pool
		statusCode int
		readyCode  int
	}{
		{
			name:       "nil pool",
			pool:       nil,
			statusCode: http.StatusServiceUnavailable,
			readyCode:  http.StatusServiceUnavailable,
		},
		{
			name:       "no workers",
			pool:       &fakeStatusPool{},
			statusCode: http.StatusServiceUnavailable,
			readyCode:  http.StatusServiceUnavailable,
		},
		{
			name:       "inactive worker",
			pool:       &fakeStatusPool{workers: []*worker.Process{newWorker(t, fsm.StateInactive)}},
			statusCode: http.StatusServiceUnavailable,
			readyCode:  http.StatusServiceUnavailable,
		},
		{
			name:       "ready worker",
			pool:       &fakeStatusPool{workers: []*worker.Process{newWorker(t, fsm.StateReady)}},
			statusCode: http.StatusOK,
			readyCode:  http.StatusOK,
		},
		{
			name:       "working worker",
			pool:       &fakeStatusPool{workers: []*worker.Process{newWorker(t, fsm.StateWorking)}},
			statusCode: http.StatusOK,
			readyCode:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plugin{
				mu:    &sync.RWMutex{},
				gPool: tt.pool,
			}

			st, err := p.Status()
			require.NoError(t, err)
			assert.Equal(t, tt.statusCode, st.Code)

			rd, err := p.Ready()
			require.NoError(t, err)
			assert.Equal(t, tt.readyCode, rd.Code)
		})
	}
}
