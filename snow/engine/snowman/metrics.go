// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/utils/wrappers"
)

type metrics struct {
	numRequests, numBlocked, numProcessing prometheus.Gauge
	getAncestorsBlks                       prometheus.Histogram
}

// Initialize the metrics
func (m *metrics) Initialize(namespace string, registerer prometheus.Registerer) error {
	m.numRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "requests",
		Help:      "Number of outstanding block requests",
	})
	m.numBlocked = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "blocked",
		Help:      "Number of blocks that are queued to be added to consensus once dependencies are met",
	})
	m.numProcessing = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "processing",
		Help:      "Number of blocks that are currently processing in the engine",
	})
	m.getAncestorsBlks = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "get_ancestors_blks",
		Help:      "The number of blocks fetched in a call to GetAncestors",
		Buckets: []float64{
			0,
			1,
			5,
			10,
			100,
			500,
			1000,
			1500,
			2000,
		},
	})

	errs := wrappers.Errs{}
	errs.Add(
		registerer.Register(m.numRequests),
		registerer.Register(m.numBlocked),
		registerer.Register(m.numProcessing),
		registerer.Register(m.getAncestorsBlks),
	)
	return errs.Err
}
