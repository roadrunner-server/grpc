package grpc

import (
	"net/http"

	"github.com/roadrunner-server/api-plugins/v6/status"
)

// Status return status of the particular plugin
func (p *Plugin) Status() (*status.Status, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.gPool == nil {
		return &status.Status{
			Code: http.StatusServiceUnavailable,
		}, nil
	}

	workers := p.gPool.Workers()

	for i := range workers {
		if workers[i].State().IsActive() {
			return &status.Status{
				Code: http.StatusOK,
			}, nil
		}
	}
	// if there are no workers, threat this as error
	return &status.Status{
		Code: http.StatusServiceUnavailable,
	}, nil
}

// Ready returns readiness status of the particular plugin.
func (p *Plugin) Ready() (*status.Status, error) {
	return p.Status()
}
