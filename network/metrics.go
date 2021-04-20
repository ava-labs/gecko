// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

type messageMetrics struct {
	receivedBytes, sentBytes, numSent, numFailed, numReceived prometheus.Counter
}

func (mm *messageMetrics) initialize(msgType Op, registerer prometheus.Registerer) error {
	mm.numSent = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: constants.PlatformName,
		Name:      fmt.Sprintf("%s_sent", msgType),
		Help:      fmt.Sprintf("Number of %s messages sent over the network", msgType),
	})
	mm.numFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: constants.PlatformName,
		Name:      fmt.Sprintf("%s_failed", msgType),
		Help:      fmt.Sprintf("Number of %s messages that failed to be sent over the network", msgType),
	})
	mm.numReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: constants.PlatformName,
		Name:      fmt.Sprintf("%s_received", msgType),
		Help:      fmt.Sprintf("Number of %s messages received from the network", msgType),
	})

	mm.receivedBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: constants.PlatformName,
		Name:      fmt.Sprintf("%s_received_bytes", msgType),
		Help:      fmt.Sprintf("Number of bytes of %s messages received from the network", msgType),
	})
	mm.sentBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: constants.PlatformName,
		Name:      fmt.Sprintf("%s_sent_bytes", msgType),
		Help:      fmt.Sprintf("Size of bytes of %s messages received from the network", msgType),
	})

	errs := wrappers.Errs{}
	errs.Add(
		registerer.Register(mm.numSent),
		registerer.Register(mm.numFailed),
		registerer.Register(mm.numReceived),
		registerer.Register(mm.receivedBytes),
		registerer.Register(mm.sentBytes),
	)

	return errs.Err
}

type metrics struct {
	numPeers                 prometheus.Gauge
	timeSinceLastMsgSent     prometheus.Gauge
	timeSinceLastMsgReceived prometheus.Gauge
	sendQueuePortionFull     prometheus.Gauge
	sendFailRate             prometheus.Gauge

	getVersion, version,
	getPeerlist, peerlist,
	ping, pong,
	getAcceptedFrontier, acceptedFrontier,
	getAccepted, accepted,
	get, getAncestors, put, multiPut,
	pushQuery, pullQuery, chits messageMetrics
}

func (m *metrics) initialize(registerer prometheus.Registerer) error {
	// Set up metrics
	m.numPeers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: constants.PlatformName,
		Name:      "peers",
		Help:      "Number of network peers",
	})
	m.timeSinceLastMsgReceived = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: constants.PlatformName,
		Name:      "time_since_last_msg_received",
		Help:      "Time since the last msg was received in milliseconds",
	})
	m.timeSinceLastMsgSent = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: constants.PlatformName,
		Name:      "time_since_last_msg_sent",
		Help:      "Time since the last msg was sent in milliseconds",
	})
	m.sendQueuePortionFull = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: constants.PlatformName,
		Name:      "send_queue_portion_full",
		Help:      "Percentage of use in Send Queue",
	})
	m.sendFailRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: constants.PlatformName,
		Name:      "send_fail_rate",
		Help:      "Portion of messages that recently failed to be sent over the network",
	})

	errs := wrappers.Errs{}
	errs.Add(
		registerer.Register(m.numPeers),
		registerer.Register(m.timeSinceLastMsgReceived),
		registerer.Register(m.timeSinceLastMsgSent),
		registerer.Register(m.sendQueuePortionFull),
		registerer.Register(m.sendFailRate),

		m.getVersion.initialize(GetVersion, registerer),
		m.version.initialize(Version, registerer),
		m.getPeerlist.initialize(GetPeerList, registerer),
		m.peerlist.initialize(PeerList, registerer),
		m.ping.initialize(Ping, registerer),
		m.pong.initialize(Pong, registerer),
		m.getAcceptedFrontier.initialize(GetAcceptedFrontier, registerer),
		m.acceptedFrontier.initialize(AcceptedFrontier, registerer),
		m.getAccepted.initialize(GetAccepted, registerer),
		m.accepted.initialize(Accepted, registerer),
		m.get.initialize(Get, registerer),
		m.getAncestors.initialize(GetAncestors, registerer),
		m.put.initialize(Put, registerer),
		m.multiPut.initialize(MultiPut, registerer),
		m.pushQuery.initialize(PushQuery, registerer),
		m.pullQuery.initialize(PullQuery, registerer),
		m.chits.initialize(Chits, registerer),
	)
	return errs.Err
}

func (m *metrics) message(msgType Op) *messageMetrics {
	switch msgType {
	case GetVersion:
		return &m.getVersion
	case Version:
		return &m.version
	case GetPeerList:
		return &m.getPeerlist
	case PeerList:
		return &m.peerlist
	case Ping:
		return &m.ping
	case Pong:
		return &m.pong
	case GetAcceptedFrontier:
		return &m.getAcceptedFrontier
	case AcceptedFrontier:
		return &m.acceptedFrontier
	case GetAccepted:
		return &m.getAccepted
	case Accepted:
		return &m.accepted
	case Get:
		return &m.get
	case GetAncestors:
		return &m.getAncestors
	case Put:
		return &m.put
	case MultiPut:
		return &m.multiPut
	case PushQuery:
		return &m.pushQuery
	case PullQuery:
		return &m.pullQuery
	case Chits:
		return &m.chits
	default:
		return nil
	}
}
