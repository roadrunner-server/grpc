package grpc

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/roadrunner-server/sdk/v3/metrics"
	"github.com/roadrunner-server/sdk/v3/state/process"
)

// Informer used to get workers from particular plugin or set of plugins
type Informer interface {
	Workers() []*process.State
}

func (p *Plugin) MetricsCollector() []prometheus.Collector {
	// p - implements Exporter interface (workers)
	// other - request duration and count
	return []prometheus.Collector{p.statsExporter, p.requestCounter, p.requestDuration, p.queueSize}
}

const (
	namespace = "rr_grpc"
)

func newStatsExporter(stats Informer) *metrics.StatsExporter {
	return &metrics.StatsExporter{
		TotalMemoryDesc:  prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "workers_memory_bytes"), "Memory usage by workers", nil, nil),
		StateDesc:        prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "worker_state"), "Worker current state", []string{"state", "pid"}, nil),
		WorkerMemoryDesc: prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "worker_memory_bytes"), "Worker current memory usage", []string{"pid"}, nil),
		TotalWorkersDesc: prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "total_workers"), "Total number of workers used by the plugin", nil, nil),
		WorkersReady:     prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "workers_ready"), "Workers currently in ready state", nil, nil),
		WorkersWorking:   prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "workers_working"), "Workers currently in working state", nil, nil),
		WorkersInvalid:   prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "workers_invalid"), "Workers currently in invalid,killing,destroyed,errored,inactive states", nil, nil),
		Workers:          stats,
	}
}
