// (c) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/prometheus/client_golang/prometheus"
)

// routerMetrics about router messages
type routerMetrics struct {
	outstandingRequests   prometheus.Gauge
	msgDropRate           prometheus.Gauge
	longestRunningRequest prometheus.Gauge
}

func newRouterMetrics(namespace string, registerer prometheus.Registerer) (*routerMetrics, error) {
	rMetrics := &routerMetrics{}
	rMetrics.outstandingRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "outstanding_requests",
			Help:      "Number of outstanding requests (all types)",
		},
	)
	rMetrics.msgDropRate = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "msg_drop_rate",
			Help:      "Rate of messages dropped",
		},
	)
	rMetrics.longestRunningRequest = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "longest_running_request",
			Help:      "Time the longest request took in milliseconds",
		},
	)

	errs := wrappers.Errs{}
	errs.Add(
		registerer.Register(rMetrics.outstandingRequests),
		registerer.Register(rMetrics.msgDropRate),
		registerer.Register(rMetrics.longestRunningRequest),
	)
	return rMetrics, errs.Err
}
