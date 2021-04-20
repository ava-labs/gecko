// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/networking/tracker"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// DefaultMaxNonStakerPendingMsgs is the default number of messages that can be taken from
	// the shared message pool by a single node
	DefaultMaxNonStakerPendingMsgs uint32 = 20
	// DefaultStakerPortion is the default portion of resources to reserve for stakers
	DefaultStakerPortion float64 = 0.375
)

// MsgManager manages incoming messages. It should be called when an incoming message
// is ready to be processed and when an incoming message is processed. We call the
// message "pending" if it has been received but not processed.
type MsgManager interface {
	// AddPending marks that there is a message from [vdr] ready to be processed.
	// Returns true if the message will eventually be processed.
	AddPending(ids.ShortID) bool
	// Called when we process a message from the given peer
	RemovePending(ids.ShortID)
	Utilization(ids.ShortID) float64
}

// msgManager implements MsgManager
type msgManager struct {
	log                            logging.Logger
	vdrs                           validators.Set
	maxNonStakerPendingMsgs        uint32
	poolMessages, reservedMessages uint32
	msgTracker                     tracker.CountingTracker
	stakerCPUPortion               float64
	cpuTracker                     tracker.TimeTracker
	clock                          timer.Clock
	metrics                        msgManagerMetrics
}

// NewMsgManager returns a new MsgManager
// [vdrs] is the network validator set
// [msgTracker] tracks how many messages we've received from each peer
// [cpuTracker] tracks how much time we spend processing messages from each peer
// [maxPendingMsgs] is the maximum number of pending messages (those we have
//   received but not processed.)
// [maxNonStakerPendingMsgs] is the maximum number of pending messages from non-validators.
func NewMsgManager(
	vdrs validators.Set,
	log logging.Logger,
	msgTracker tracker.CountingTracker,
	cpuTracker tracker.TimeTracker,
	maxPendingMsgs,
	maxNonStakerPendingMsgs uint32,
	stakerMsgPortion,
	stakerCPUPortion float64,
	metricsNamespace string,
	metricsRegisterer prometheus.Registerer,
) (MsgManager, error) {
	// Number of messages reserved for stakers vs. non-stakers
	reservedMessages := uint32(stakerMsgPortion * float64(maxPendingMsgs))
	poolMessages := maxPendingMsgs - reservedMessages
	metrics := msgManagerMetrics{}
	if err := metrics.initialize(metricsNamespace, metricsRegisterer); err != nil {
		return nil, err
	}

	return &msgManager{
		vdrs:                    vdrs,
		msgTracker:              msgTracker,
		cpuTracker:              cpuTracker,
		log:                     log,
		reservedMessages:        reservedMessages,
		poolMessages:            poolMessages,
		maxNonStakerPendingMsgs: maxNonStakerPendingMsgs,
		stakerCPUPortion:        stakerCPUPortion,
		metrics:                 metrics,
	}, nil
}

// AddPending marks that there is a message from [vdr] ready to be processed.
// Return true if the message was added to the processing list.
func (rm *msgManager) AddPending(vdr ids.ShortID) bool {
	// Attempt to take the message from the pool
	outstandingPoolMessages := rm.msgTracker.PoolCount()
	totalPeerMessages, peerPoolMessages := rm.msgTracker.OutstandingCount(vdr)

	rm.metrics.poolMsgsAvailable.Set(float64(rm.poolMessages - outstandingPoolMessages))
	// True if the all the messages in the at-large message pool have been used
	poolEmpty := outstandingPoolMessages >= rm.poolMessages
	// True if this node has used the maximum number of messages from the at-large message pool
	poolAllocUsed := peerPoolMessages >= rm.maxNonStakerPendingMsgs
	if !poolEmpty && !poolAllocUsed {
		// This node can use a message from the at-large message pool
		rm.msgTracker.AddPool(vdr)
		return true
	}

	// Attempt to take the message from the individual allotment
	weight, isStaker := rm.vdrs.GetWeight(vdr)
	if !isStaker {
		if poolEmpty {
			rm.metrics.throttledPoolEmpty.Inc()
		} else if poolAllocUsed {
			rm.metrics.throttledPoolAllocExhausted.Inc()
		}
		rm.log.Verbo("Throttling message from non-staker %s. %d/%d.", vdr, peerPoolMessages, rm.poolMessages)
		return false
	}
	totalWeight := rm.vdrs.Weight()
	stakerPortion := float64(weight) / float64(totalWeight)
	messageAllotment := uint32(stakerPortion * float64(rm.reservedMessages))
	messageCount := totalPeerMessages - peerPoolMessages
	// Allow at least one message per staker, even when staking
	// portion rounds message allotment down to 0.
	if messageCount <= messageAllotment {
		rm.msgTracker.Add(vdr)
		return true
	}
	rm.metrics.throttledVdrAllocExhausted.Inc()

	rm.log.Debug("Throttling message from staker %s. %d/%d. %d/%d.", vdr, messageCount, messageAllotment, peerPoolMessages, rm.poolMessages)
	return false
}

// RemovePending marks that a message from [vdr] has been processed.
func (rm *msgManager) RemovePending(vdr ids.ShortID) {
	rm.msgTracker.Remove(vdr)
}

// Utilization returns the percentage of expected utilization
// for [vdr] to determine message priority
func (rm *msgManager) Utilization(vdr ids.ShortID) float64 {
	currentTime := rm.clock.Time()
	vdrUtilization := rm.cpuTracker.Utilization(vdr, currentTime)
	numSpenders := rm.cpuTracker.Len()
	poolAllotment := (1 - rm.stakerCPUPortion) / float64(numSpenders)

	weight, exists := rm.vdrs.GetWeight(vdr)
	if !exists {
		return vdrUtilization / poolAllotment
	}

	totalWeight := rm.vdrs.Weight()
	stakerPortion := float64(weight) / float64(totalWeight)
	stakerAllotment := stakerPortion*rm.stakerCPUPortion + poolAllotment

	return vdrUtilization / stakerAllotment
}

type msgManagerMetrics struct {
	poolMsgsAvailable prometheus.Gauge
	throttledPoolEmpty,
	throttledPoolAllocExhausted,
	throttledVdrAllocExhausted prometheus.Counter
}

func (m *msgManagerMetrics) initialize(namespace string, registerer prometheus.Registerer) error {
	errs := wrappers.Errs{}
	m.poolMsgsAvailable = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "pool_msgs_available",
		Help:      "Number of available messages in the at-large pending message pool",
	})
	if err := registerer.Register(m.poolMsgsAvailable); err != nil {
		errs.Add(fmt.Errorf("failed to register throttled statistics due to %w", err))
	}

	m.throttledPoolEmpty = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "throttled_pool_empty",
		Help:      "Number of incoming messages dropped because at-large pending message pool is empty",
	})
	if err := registerer.Register(m.throttledPoolEmpty); err != nil {
		errs.Add(fmt.Errorf("failed to register throttled statistics due to %w", err))
	}

	m.throttledPoolAllocExhausted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "throttled_pool_alloc_exhausted",
		Help:      "Number of incoming messages dropped because a non-validator used the max number of messages from the at-large pool",
	})
	if err := registerer.Register(m.throttledPoolAllocExhausted); err != nil {
		errs.Add(fmt.Errorf("failed to register throttled statistics due to %w", err))
	}

	m.throttledVdrAllocExhausted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "throttled_validator_alloc_exhausted",
		Help:      "Number of incoming messages dropped because a validator used the max number of pending messages allocated to them",
	})
	if err := registerer.Register(m.throttledVdrAllocExhausted); err != nil {
		errs.Add(fmt.Errorf("failed to register throttled statistics due to %w", err))
	}
	return errs.Err
}
