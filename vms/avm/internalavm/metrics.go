// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/utils/wrappers"
)

func newCallsMetric(namespace, name string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      fmt.Sprintf("%s_calls", name),
		Help:      fmt.Sprintf("Number of times %s has been called", name),
	})
}

type metrics struct {
	numBootstrappingCalls, numBootstrappedCalls, numCreateHandlersCalls,
	numPendingTxsCalls, numParseTxCalls, numGetTxCalls prometheus.Counter

	numTxRefreshes, numTxRefreshHits, numTxRefreshMisses prometheus.Counter
}

func (m *metrics) Initialize(
	namespace string,
	registerer prometheus.Registerer,
) error {
	m.numBootstrappingCalls = newCallsMetric(namespace, "bootstrapping")
	m.numBootstrappedCalls = newCallsMetric(namespace, "bootstrapped")
	m.numCreateHandlersCalls = newCallsMetric(namespace, "create_handlers")
	m.numPendingTxsCalls = newCallsMetric(namespace, "pending_txs")
	m.numParseTxCalls = newCallsMetric(namespace, "parse_tx")
	m.numGetTxCalls = newCallsMetric(namespace, "get_tx")

	m.numTxRefreshes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tx_refreshes",
		Help:      "Number of times unique txs have been refreshed",
	})
	m.numTxRefreshHits = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tx_refresh_hits",
		Help:      "Number of times unique txs have not been unique, but were cached",
	})
	m.numTxRefreshMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tx_refresh_misses",
		Help:      "Number of times unique txs have not been unique and weren't cached",
	})

	errs := wrappers.Errs{}
	errs.Add(
		registerer.Register(m.numBootstrappingCalls),
		registerer.Register(m.numBootstrappedCalls),
		registerer.Register(m.numCreateHandlersCalls),
		registerer.Register(m.numPendingTxsCalls),
		registerer.Register(m.numParseTxCalls),
		registerer.Register(m.numGetTxCalls),
		registerer.Register(m.numTxRefreshes),
		registerer.Register(m.numTxRefreshHits),
		registerer.Register(m.numTxRefreshMisses),
	)
	return errs.Err
}
