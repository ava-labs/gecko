// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	defaultRequestHelpMsg = "Time spent processing this request in nanoseconds"
)

func initHistogram(namespace, name string, registerer prometheus.Registerer, errs *wrappers.Errs) prometheus.Histogram {
	histogram := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      name,
			Help:      defaultRequestHelpMsg,
			Buckets:   utils.NanosecondsBuckets,
		})

	if err := registerer.Register(histogram); err != nil {
		errs.Add(fmt.Errorf("failed to register %s statistics due to %w", name, err))
	}
	return histogram
}

type handlerMetrics struct {
	namespace        string
	registerer       prometheus.Registerer
	pending          prometheus.Gauge
	dropped, expired prometheus.Counter
	getAcceptedFrontier, acceptedFrontier, getAcceptedFrontierFailed,
	getAccepted, accepted, getAcceptedFailed,
	getAncestors, multiPut, getAncestorsFailed,
	get, put, getFailed,
	pushQuery, pullQuery, chits, queryFailed,
	connected, disconnected,
	notify,
	gossip,
	cpu,
	shutdown prometheus.Histogram
}

// Initialize implements the Engine interface
func (m *handlerMetrics) Initialize(namespace string, registerer prometheus.Registerer) error {
	m.namespace = namespace
	m.registerer = registerer
	errs := wrappers.Errs{}

	m.pending = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "pending",
		Help:      "Number of pending events",
	})
	if err := registerer.Register(m.pending); err != nil {
		errs.Add(fmt.Errorf("failed to register pending statistics due to %w", err))
	}

	m.dropped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "dropped",
		Help:      "Number of dropped events",
	})

	if err := registerer.Register(m.dropped); err != nil {
		errs.Add(fmt.Errorf("failed to register dropped statistics due to %w", err))
	}

	m.expired = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "expired",
		Help:      "Number of expired events",
	})
	if err := registerer.Register(m.expired); err != nil {
		errs.Add(fmt.Errorf("failed to register expired statistics due to %w", err))
	}

	m.getAcceptedFrontier = initHistogram(namespace, "get_accepted_frontier", registerer, &errs)
	m.acceptedFrontier = initHistogram(namespace, "accepted_frontier", registerer, &errs)
	m.getAcceptedFrontierFailed = initHistogram(namespace, "get_accepted_frontier_failed", registerer, &errs)
	m.getAccepted = initHistogram(namespace, "get_accepted", registerer, &errs)
	m.accepted = initHistogram(namespace, "accepted", registerer, &errs)
	m.getAcceptedFailed = initHistogram(namespace, "get_accepted_failed", registerer, &errs)
	m.getAncestors = initHistogram(namespace, "get_ancestors", registerer, &errs)
	m.multiPut = initHistogram(namespace, "multi_put", registerer, &errs)
	m.getAncestorsFailed = initHistogram(namespace, "get_ancestors_failed", registerer, &errs)
	m.get = initHistogram(namespace, "get", registerer, &errs)
	m.put = initHistogram(namespace, "put", registerer, &errs)
	m.getFailed = initHistogram(namespace, "get_failed", registerer, &errs)
	m.pushQuery = initHistogram(namespace, "push_query", registerer, &errs)
	m.pullQuery = initHistogram(namespace, "pull_query", registerer, &errs)
	m.chits = initHistogram(namespace, "chits", registerer, &errs)
	m.queryFailed = initHistogram(namespace, "query_failed", registerer, &errs)
	m.connected = initHistogram(namespace, "connected", registerer, &errs)
	m.disconnected = initHistogram(namespace, "disconnected", registerer, &errs)
	m.notify = initHistogram(namespace, "notify", registerer, &errs)
	m.gossip = initHistogram(namespace, "gossip", registerer, &errs)

	m.cpu = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "cpu_utilization",
		Help:      "Time this handler's engine spent processing messages for a single CPU interval in milliseconds",
		Buckets:   utils.MillisecondsBuckets,
	})
	if err := registerer.Register(m.cpu); err != nil {
		errs.Add(fmt.Errorf("failed to register shutdown statistics due to %w", err))
	}
	m.shutdown = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "shutdown",
		Help:      "Time spent in the process of shutting down in nanoseconds",
		Buckets:   utils.NanosecondsBuckets,
	})
	if err := registerer.Register(m.shutdown); err != nil {
		errs.Add(fmt.Errorf("failed to register shutdown statistics due to %w", err))
	}

	return errs.Err
}

func (m *handlerMetrics) registerTierStatistics(tier int) (prometheus.Gauge, prometheus.Histogram, error) {
	errs := wrappers.Errs{}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: m.namespace,
		Name:      fmt.Sprintf("tier_%d", tier),
		Help:      fmt.Sprintf("Number of pending messages on tier %d of the multi-level message queue", tier),
	})
	if err := m.registerer.Register(gauge); err != nil {
		errs.Add(fmt.Errorf("failed to register tier_%d statistics due to %w", tier, err))
	}

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: m.namespace,
		Name:      fmt.Sprintf("tier_%d_wait_time", tier),
		Help:      fmt.Sprintf("Amount of time a message waits on tier %d queue before being processed", tier),
		Buckets:   utils.NanosecondsBuckets,
	})
	if err := m.registerer.Register(histogram); err != nil {
		errs.Add(fmt.Errorf("failed to register tier_%d_wait_time statistics due to %w", tier, err))
	}
	return gauge, histogram, errs.Err
}

func (m *handlerMetrics) getMSGHistogram(msg constants.MsgType) prometheus.Histogram {
	switch msg {
	case constants.GetAcceptedFrontierMsg:
		return m.getAcceptedFrontier
	case constants.AcceptedFrontierMsg:
		return m.acceptedFrontier
	case constants.GetAcceptedFrontierFailedMsg:
		return m.getAcceptedFrontierFailed
	case constants.GetAcceptedMsg:
		return m.getAccepted
	case constants.AcceptedMsg:
		return m.accepted
	case constants.GetAcceptedFailedMsg:
		return m.getAcceptedFailed
	case constants.GetAncestorsMsg:
		return m.getAncestors
	case constants.GetAncestorsFailedMsg:
		return m.getAncestorsFailed
	case constants.MultiPutMsg:
		return m.multiPut
	case constants.GetMsg:
		return m.get
	case constants.GetFailedMsg:
		return m.getFailed
	case constants.PutMsg:
		return m.put
	case constants.PushQueryMsg:
		return m.pushQuery
	case constants.PullQueryMsg:
		return m.pullQuery
	case constants.QueryFailedMsg:
		return m.queryFailed
	case constants.ChitsMsg:
		return m.chits
	case constants.ConnectedMsg:
		return m.connected
	case constants.DisconnectedMsg:
		return m.disconnected
	default:
		panic(fmt.Sprintf("unknown message type %s", msg))
	}
}
