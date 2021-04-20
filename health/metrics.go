// (c) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// metrics reports commonly used health check metrics.
type metrics struct {
	// log reports anomalous events.
	log logging.Logger

	// failingChecks keeps track of the number of check failing
	failingChecks prometheus.Gauge
}

func newMetrics(log logging.Logger, namespace string, registerer prometheus.Registerer) (*metrics, error) {
	metrics := &metrics{
		log: log,
		failingChecks: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "health_checks_failing",
			Help:      "number of currently failing health checks",
		}),
	}
	return metrics, registerer.Register(metrics.failingChecks)
}

// healthy handles the metrics for the healthy cases
func (m *metrics) healthy() {
	m.failingChecks.Dec()
}

// unHealthy handles the metrics for the unhealthy cases
func (m *metrics) unHealthy() {
	m.failingChecks.Inc()
}
