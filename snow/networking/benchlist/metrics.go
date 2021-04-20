// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package benchlist

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	numBenched, weightBenched prometheus.Gauge
}

// Initialize implements the Engine interface
func (m *metrics) Initialize(registerer prometheus.Registerer, namespace string) error {
	benchNamespace := fmt.Sprintf("%s_benchlist", namespace)

	m.numBenched = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: benchNamespace,
		Name:      "benched_num",
		Help:      "Number of currently benched validators",
	})
	if err := registerer.Register(m.numBenched); err != nil {
		return fmt.Errorf("failed to register num benched statistics due to %w", err)
	}

	m.weightBenched = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: benchNamespace,
		Name:      "benched_weight",
		Help:      "Weight of currently benched validators",
	})
	if err := registerer.Register(m.weightBenched); err != nil {
		return fmt.Errorf("failed to register weight benched statistics due to %w", err)
	}

	return nil
}
