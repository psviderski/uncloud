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
	DNSQuery = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace, Subsystem: "dns",
		Name: "query_total",
		Help: "Counter of DNS queries.",
	}, []string{"internal", "status"})
)

const (
	Err = "err"
	Ok  = "ok"
)

// Status returns "ok" is err is nil, otherwise "err".
func Status(err error) string {
	if err != nil {
		return Err
	}
	return Ok
}

const Namespace = "uncloud"
