// Package metrics declares all metrics Uncloud uses.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Version = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace, Subsystem: "uncloudd",
		Name: "build_info",
		Help: "Build information.",
	}, []string{"version"})

	ContainerExec = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace, Subsystem: "container",
		Name: "exec_total",
		Help: "Counter of container executions performed.",
	}, []string{"op", "status"})

	DNSQuery = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace, Subsystem: "dns",
		Name: "query_total",
		Help: "Counter of DNS queries seen.",
	}, []string{"internal", "status"})
)

const Namespace = "uncloud"
