package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const Namespace = "helm_watch"

type Registry struct {
	prometheus.Registerer
	prometheus.Gatherer
}

func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return &Registry{
		Registerer: reg,
		Gatherer:   reg,
	}
}
