package grpc

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/roadrunner-server/api/v2/plugins/informer"
)

func (p *Plugin) MetricsCollector() []prometheus.Collector {
	// p - implements Exporter interface (workers)
	// other - request duration and count
	return []prometheus.Collector{p.statsExporter}
}

const (
	namespace = "rr_grpc"
)

type statsExporter struct {
	workers       informer.Informer
	workersMemory uint64

	totalMemoryDesc  *prometheus.Desc
	stateDesc        *prometheus.Desc
	workerMemoryDesc *prometheus.Desc
	totalWorkersDesc *prometheus.Desc
}

func newStatsExporter(stats informer.Informer) *statsExporter {
	return &statsExporter{
		workers:       stats,
		workersMemory: 0,

		totalMemoryDesc:  prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "workers_memory_bytes"), "Memory usage by JOBS workers", nil, nil),
		workerMemoryDesc: prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "worker_memory_bytes"), "Worker current memory usage", []string{"pid"}, nil),
		stateDesc:        prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "worker_state"), "Worker current state", []string{"state", "pid"}, nil),
		totalWorkersDesc: prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "total_workers"), "Total number of workers used by the JOBS plugin", nil, nil),
	}
}

func (se *statsExporter) Describe(d chan<- *prometheus.Desc) {
	// send description
	d <- se.totalMemoryDesc
	d <- se.stateDesc
	d <- se.workerMemoryDesc
	d <- se.totalWorkersDesc
}

func (se *statsExporter) Collect(ch chan<- prometheus.Metric) {
	// get the copy of the processes
	workers := se.workers.Workers()

	// cumulative RSS memory in bytes
	var cum uint64

	// collect the memory
	for i := 0; i < len(workers); i++ {
		cum += workers[i].MemoryUsage

		ch <- prometheus.MustNewConstMetric(se.stateDesc, prometheus.GaugeValue, 0, workers[i].Status, strconv.Itoa(workers[i].Pid))
		ch <- prometheus.MustNewConstMetric(se.workerMemoryDesc, prometheus.GaugeValue, float64(workers[i].MemoryUsage), strconv.Itoa(workers[i].Pid))
	}

	// send the values to the prometheus
	ch <- prometheus.MustNewConstMetric(se.totalWorkersDesc, prometheus.GaugeValue, float64(len(workers)))
	ch <- prometheus.MustNewConstMetric(se.totalMemoryDesc, prometheus.GaugeValue, float64(cum))
}
