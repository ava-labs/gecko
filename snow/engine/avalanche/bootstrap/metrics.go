// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/utils/wrappers"
)

type metrics struct {
	numFetchedVts, numDroppedVts, numAcceptedVts,
	numFetchedTxs, numDroppedTxs, numAcceptedTxs prometheus.Counter
}

// Initialize implements the Engine interface
func (m *metrics) Initialize(
	namespace string,
	registerer prometheus.Registerer,
) error {
	m.numFetchedVts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "fetched_vts",
		Help:      "Number of vertices fetched during bootstrapping",
	})
	m.numDroppedVts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dropped_vts",
		Help:      "Number of vertices dropped during bootstrapping",
	})
	m.numAcceptedVts = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "accepted_vts",
		Help:      "Number of vertices accepted during bootstrapping",
	})

	m.numFetchedTxs = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "fetched_txs",
		Help:      "Number of transactions fetched during bootstrapping",
	})
	m.numDroppedTxs = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dropped_txs",
		Help:      "Number of transactions dropped during bootstrapping",
	})
	m.numAcceptedTxs = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "accepted_txs",
		Help:      "Number of transactions accepted during bootstrapping",
	})

	errs := wrappers.Errs{}
	errs.Add(
		registerer.Register(m.numFetchedVts),
		registerer.Register(m.numDroppedVts),
		registerer.Register(m.numAcceptedVts),
		registerer.Register(m.numFetchedTxs),
		registerer.Register(m.numDroppedTxs),
		registerer.Register(m.numAcceptedTxs),
	)
	return errs.Err
}
