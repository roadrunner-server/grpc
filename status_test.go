package grpc

import (
	"context"
	"net/http"
	"os/exec"
	"sync"
	"testing"

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

func newStatusPlugin(workers ...*worker.Process) *Plugin {
	return &Plugin{
		mu:    &sync.RWMutex{},
		gPool: &fakeStatusPool{workers: workers},
	}
}

func TestPluginStatus(t *testing.T) {
	// No workers -> service unavailable.
	st, err := newStatusPlugin().Status()
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, st.Code)

	// At least one active (ready) worker -> OK.
	st, err = newStatusPlugin(newWorker(t, fsm.StateReady)).Status()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, st.Code)
}

func TestPluginReady(t *testing.T) {
	// No workers -> service unavailable.
	st, err := newStatusPlugin().Ready()
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, st.Code)

	// A ready worker -> OK.
	st, err = newStatusPlugin(newWorker(t, fsm.StateReady)).Ready()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, st.Code)

	// A worker that is active but only working (not ready) -> not ready.
	st, err = newStatusPlugin(newWorker(t, fsm.StateWorking)).Ready()
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, st.Code)
}
