// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import (
	"math"
	"sync"
	"time"

	"github.com/ava-labs/avalanchego/utils"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/networking/tracker"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/uptime"
)

// Requirement: A set of nodes spamming messages (potentially costly) shouldn't
//              impact other node's queries.

// Requirement: The staked validators should be able to maintain liveness, even
//              if that requires sacrificing liveness of the non-staked
//              validators. This ensures the network keeps moving forwards.

// Idea: There is 1 second of cpu time per second, divide that out into the
//       stakers (potentially by staking weight).

// Idea: Non-stakers are treated as if they have the same identity.

// Idea: Beacons should receive special treatment.

// Problem: Our queues need to have bounded size, so we need to drop messages.
//          When should we be dropping messages? If we only drop based on the
//          queue being full, then a peer can spam messages to fill the queue
//          causing other peers' messages to drop.
// Answer: Drop messages if the peer has too many outstanding messages. (Could be
//         weighted by the size of the queue + stake amount)

// Problem: How should we prioritize peers? If we are already picking which
//          level of the queue the peer's messages are going into, then the
//          internal queue can just be FIFO.
// Answer: Somehow we track the cpu time of the peer (WMA). Based on that, we
//         place the message into a corresponding bucket. When pulling the
//         message from the bucket, we check to see if the message should be
//         moved to a lower bucket. If so move the message to the lower queue
//         and process the next message.

// Structure:
//  [000%-050%] P0: Chan msg    size = 1024    CPU time per iteration = 200ms
//  [050%-075%] P1: Chan msg    size = 1024    CPU time per iteration = 150ms
//  [075%-100%] P2: Chan msg    size = 1024    CPU time per iteration = 100ms
//  [100%-125%] P3: Chan msg    size = 1024    CPU time per iteration = 050ms
//  [125%-INF%] P4: Chan msg    size = 1024    CPU time per iteration = 001ms

// 20% resources for stakers. = RE_s
// 80% resources for non-stakers. = RE_n

// Each peer is going to have calculated their expected CPU utilization.
//
// E[Staker CPU Utilization] = RE_s * weight + RE_n / NumPeers
// E[Non-Staker CPU Utilization] = RE_n / NumPeers

// Each peer is going to have calculated their max number of outstanding
// messages.
//
// Max[Staker Messages] = (RE_s * weight + RE_n / NumPeers) * MaxMessages
// Max[Non-Staker Messages] = (RE_n / NumPeers) * MaxMessages

// Problem: If everyone is part of the P0 queue, except for a couple byzantine
//          nodes. Then the byzantine nodes can take up 80% of the CPU.

// Vars to tune:
// - % reserved for stakers.
// - CPU time per buckets
// - % range of buckets
// - number of buckets
// - size of buckets
// - how to track CPU utilization of a peer
// - "MaxMessages"

const (
	maxSleepDuration = 100 * time.Millisecond
)

// Handler passes incoming messages from the network to the consensus engine
// (Actually, it receives the incoming messages from a ChainRouter, but same difference)
type Handler struct {
	metrics handlerMetrics

	validators validators.Set

	// This is the channel of messages to process
	reliableMsgsSema chan struct{}
	reliableMsgsLock sync.Mutex
	reliableMsgs     []message
	closed           chan struct{}
	msgChan          <-chan common.Message

	cpuTracker tracker.TimeTracker

	clock timer.Clock

	serviceQueue messageQueue
	msgSema      <-chan struct{}

	ctx    *snow.Context
	engine common.Engine

	toClose func()
	closing utils.AtomicBool

	delay *Delay
}

// Initialize this consensus handler
// engine must be initialized before initializing the handler
func (h *Handler) Initialize(
	engine common.Engine,
	validators validators.Set,
	msgChan <-chan common.Message,
	maxPendingMsgs uint32,
	maxNonStakerPendingMsgs uint32,
	stakerMsgPortion,
	stakerCPUPortion float64,
	namespace string,
	metrics prometheus.Registerer,
	delay *Delay,
) error {
	h.ctx = engine.Context()
	if err := h.metrics.Initialize(namespace, metrics); err != nil {
		h.ctx.Log.Warn("initializing handler metrics errored with: %s", err)
	}
	h.reliableMsgsSema = make(chan struct{}, 1)
	h.closed = make(chan struct{})
	h.msgChan = msgChan

	// Defines the maximum current percentage of expected CPU utilization for
	// a message to be placed in the queue at the corresponding index
	consumptionRanges := []float64{
		0.125,
		0.5,
		1,
		math.MaxFloat64,
	}

	cpuInterval := defaultCPUInterval
	// Defines the percentage of CPU time allotted to processing messages
	// from the bucket at the corresponding index.
	consumptionAllotments := []time.Duration{
		cpuInterval / 4,
		cpuInterval / 4,
		cpuInterval / 4,
		cpuInterval / 4,
	}

	h.cpuTracker = tracker.NewCPUTracker(uptime.IntervalFactory{}, cpuInterval)
	msgTracker := tracker.NewMessageTracker()
	msgManager, err := NewMsgManager(
		validators,
		h.ctx.Log,
		msgTracker,
		h.cpuTracker,
		maxPendingMsgs,
		maxNonStakerPendingMsgs,
		stakerMsgPortion,
		stakerCPUPortion,
		namespace,
		metrics,
	)
	if err != nil {
		return err
	}

	h.serviceQueue, h.msgSema = newMultiLevelQueue(
		msgManager,
		consumptionRanges,
		consumptionAllotments,
		maxPendingMsgs,
		h.ctx.Log,
		&h.metrics,
	)
	h.engine = engine
	h.validators = validators
	h.delay = delay
	return nil
}

// Context of this Handler
func (h *Handler) Context() *snow.Context { return h.engine.Context() }

// Engine returns the engine this handler dispatches to
func (h *Handler) Engine() common.Engine { return h.engine }

// SetEngine sets the engine for this handler to dispatch to
func (h *Handler) SetEngine(engine common.Engine) { h.engine = engine }

// Dispatch waits for incoming messages from the network
// and, when they arrive, sends them to the consensus engine
func (h *Handler) Dispatch() {
	defer h.shutdownDispatch()

	for {
		select {
		case _, ok := <-h.msgSema:
			if !ok {
				// the msgSema channel has been closed, so this dispatcher should exit
				return
			}

			msg, err := h.serviceQueue.PopMessage()
			if err != nil {
				h.ctx.Log.Warn("Could not pop messsage from service queue")
				continue
			}
			if !msg.deadline.IsZero() && h.clock.Time().After(msg.deadline) {
				h.ctx.Log.Verbo("Dropping message due to likely timeout: %s", msg)
				h.metrics.dropped.Inc()
				h.metrics.expired.Inc()
				continue
			}

			h.dispatchMsg(msg)
		case <-h.reliableMsgsSema:
			// get all the reliable messages
			h.reliableMsgsLock.Lock()
			msgs := h.reliableMsgs
			h.reliableMsgs = nil
			h.reliableMsgsLock.Unlock()

			// fire all the reliable messages
			for _, msg := range msgs {
				h.metrics.pending.Dec()
				h.dispatchMsg(msg)
			}
		case msg := <-h.msgChan:
			// handle a message from the VM
			h.dispatchMsg(message{messageType: constants.NotifyMsg, notification: msg})
		}

		if h.closing.GetValue() {
			return
		}
	}
}

// Dispatch a message to the consensus engine.
func (h *Handler) dispatchMsg(msg message) {
	// If messages should be delayed to this chain, hold the messages, but check
	// to see if the chain should be closed at least every [maxSleepDuration].
	for {
		if h.closing.GetValue() {
			h.ctx.Log.Debug("dropping message due to closing:\n%s", msg)
			h.metrics.dropped.Inc()
			return
		}
		until := time.Until(h.delay.waitUntil)
		if until <= 0 {
			break
		}
		if until > maxSleepDuration {
			time.Sleep(maxSleepDuration)
		} else {
			time.Sleep(until)
		}
	}

	startTime := h.clock.Time()

	h.ctx.Lock.Lock()
	defer h.ctx.Lock.Unlock()

	if msg.IsPeriodic() {
		h.ctx.Log.Verbo("Forwarding message to consensus: %s", msg)
	} else {
		h.ctx.Log.Debug("Forwarding message to consensus: %s", msg)
	}

	var (
		err error
	)
	switch msg.messageType {
	case constants.NotifyMsg:
		err = h.engine.Notify(msg.notification)
		h.metrics.notify.Observe(float64(h.clock.Time().Sub(startTime)))
	case constants.GossipMsg:
		err = h.engine.Gossip()
		h.metrics.gossip.Observe(float64(h.clock.Time().Sub(startTime)))
	default:
		err = h.handleValidatorMsg(msg, startTime)
	}

	if msg.IsPeriodic() {
		h.ctx.Log.Verbo("Finished sending message to consensus: %s", msg.messageType)
	} else {
		h.ctx.Log.Debug("Finished sending message to consensus: %s", msg.messageType)
	}

	if err != nil {
		h.ctx.Log.Fatal("forcing chain to shutdown due to: %s", err)
		h.closing.SetValue(true)
	}
}

// GetAcceptedFrontier passes a GetAcceptedFrontier message received from the
// network to the consensus engine.
func (h *Handler) GetAcceptedFrontier(validatorID ids.ShortID, requestID uint32, deadline time.Time) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.GetAcceptedFrontierMsg,
		validatorID: validatorID,
		requestID:   requestID,
		deadline:    deadline,
		received:    h.clock.Time(),
	})
}

// AcceptedFrontier passes a AcceptedFrontier message received from the network
// to the consensus engine.
func (h *Handler) AcceptedFrontier(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType:  constants.AcceptedFrontierMsg,
		validatorID:  validatorID,
		requestID:    requestID,
		containerIDs: containerIDs,
		received:     h.clock.Time(),
	})
}

// GetAcceptedFrontierFailed passes a GetAcceptedFrontierFailed message received
// from the network to the consensus engine.
func (h *Handler) GetAcceptedFrontierFailed(validatorID ids.ShortID, requestID uint32) {
	h.sendReliableMsg(message{
		messageType: constants.GetAcceptedFrontierFailedMsg,
		validatorID: validatorID,
		requestID:   requestID,
	})
}

// GetAccepted passes a GetAccepted message received from the
// network to the consensus engine.
func (h *Handler) GetAccepted(validatorID ids.ShortID, requestID uint32, deadline time.Time, containerIDs []ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType:  constants.GetAcceptedMsg,
		validatorID:  validatorID,
		requestID:    requestID,
		deadline:     deadline,
		containerIDs: containerIDs,
		received:     h.clock.Time(),
	})
}

// Accepted passes a Accepted message received from the network to the consensus
// engine.
func (h *Handler) Accepted(validatorID ids.ShortID, requestID uint32, containerIDs []ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType:  constants.AcceptedMsg,
		validatorID:  validatorID,
		requestID:    requestID,
		containerIDs: containerIDs,
		received:     h.clock.Time(),
	})
}

// GetAcceptedFailed passes a GetAcceptedFailed message received from the
// network to the consensus engine.
func (h *Handler) GetAcceptedFailed(validatorID ids.ShortID, requestID uint32) {
	h.sendReliableMsg(message{
		messageType: constants.GetAcceptedFailedMsg,
		validatorID: validatorID,
		requestID:   requestID,
	})
}

// GetAncestors passes a GetAncestors message received from the network to the consensus engine.
func (h *Handler) GetAncestors(validatorID ids.ShortID, requestID uint32, deadline time.Time, containerID ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.GetAncestorsMsg,
		validatorID: validatorID,
		requestID:   requestID,
		deadline:    deadline,
		containerID: containerID,
		received:    h.clock.Time(),
	})
}

// MultiPut passes a MultiPut message received from the network to the consensus engine.
func (h *Handler) MultiPut(validatorID ids.ShortID, requestID uint32, containers [][]byte) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.MultiPutMsg,
		validatorID: validatorID,
		requestID:   requestID,
		containers:  containers,
		received:    h.clock.Time(),
	})
}

// GetAncestorsFailed passes a GetAncestorsFailed message to the consensus engine.
func (h *Handler) GetAncestorsFailed(validatorID ids.ShortID, requestID uint32) {
	h.sendReliableMsg(message{
		messageType: constants.GetAncestorsFailedMsg,
		validatorID: validatorID,
		requestID:   requestID,
	})
}

// Get passes a Get message received from the network to the consensus engine.
func (h *Handler) Get(validatorID ids.ShortID, requestID uint32, deadline time.Time, containerID ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.GetMsg,
		validatorID: validatorID,
		requestID:   requestID,
		deadline:    deadline,
		containerID: containerID,
		received:    h.clock.Time(),
	})
}

// Put passes a Put message received from the network to the consensus engine.
func (h *Handler) Put(validatorID ids.ShortID, requestID uint32, containerID ids.ID, container []byte) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.PutMsg,
		validatorID: validatorID,
		requestID:   requestID,
		containerID: containerID,
		container:   container,
		received:    h.clock.Time(),
	})
}

// GetFailed passes a GetFailed message to the consensus engine.
func (h *Handler) GetFailed(validatorID ids.ShortID, requestID uint32) {
	h.sendReliableMsg(message{
		messageType: constants.GetFailedMsg,
		validatorID: validatorID,
		requestID:   requestID,
	})
}

// PushQuery passes a PushQuery message received from the network to the consensus engine.
func (h *Handler) PushQuery(validatorID ids.ShortID, requestID uint32, deadline time.Time, containerID ids.ID, container []byte) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.PushQueryMsg,
		validatorID: validatorID,
		requestID:   requestID,
		deadline:    deadline,
		containerID: containerID,
		container:   container,
		received:    h.clock.Time(),
	})
}

// PullQuery passes a PullQuery message received from the network to the consensus engine.
func (h *Handler) PullQuery(validatorID ids.ShortID, requestID uint32, deadline time.Time, containerID ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType: constants.PullQueryMsg,
		validatorID: validatorID,
		requestID:   requestID,
		deadline:    deadline,
		containerID: containerID,
		received:    h.clock.Time(),
	})
}

// Chits passes a Chits message received from the network to the consensus engine.
func (h *Handler) Chits(validatorID ids.ShortID, requestID uint32, votes []ids.ID) bool {
	return h.serviceQueue.PushMessage(message{
		messageType:  constants.ChitsMsg,
		validatorID:  validatorID,
		requestID:    requestID,
		containerIDs: votes,
		received:     h.clock.Time(),
	})
}

// QueryFailed passes a QueryFailed message received from the network to the consensus engine.
func (h *Handler) QueryFailed(validatorID ids.ShortID, requestID uint32) {
	h.sendReliableMsg(message{
		messageType: constants.QueryFailedMsg,
		validatorID: validatorID,
		requestID:   requestID,
	})
}

// Connected passes a new connection notification to the consensus engine
func (h *Handler) Connected(validatorID ids.ShortID) {
	h.sendReliableMsg(message{
		messageType: constants.ConnectedMsg,
		validatorID: validatorID,
	})
}

// Disconnected passes a new connection notification to the consensus engine
func (h *Handler) Disconnected(validatorID ids.ShortID) {
	h.sendReliableMsg(message{
		messageType: constants.DisconnectedMsg,
		validatorID: validatorID,
	})
}

// Gossip passes a gossip request to the consensus engine
func (h *Handler) Gossip() {
	if !h.ctx.IsBootstrapped() {
		// Shouldn't send gossiping messages while the chain is bootstrapping
		return
	}
	h.sendReliableMsg(message{
		messageType: constants.GossipMsg,
	})
}

// Notify ...
func (h *Handler) Notify(msg common.Message) {
	h.sendReliableMsg(message{
		messageType:  constants.NotifyMsg,
		notification: msg,
	})
}

// Shutdown asynchronously shuts down the dispatcher.
// The handler should never be invoked again after calling
// Shutdown.
func (h *Handler) Shutdown() {
	h.closing.SetValue(true)
	h.serviceQueue.Shutdown()
}

func (h *Handler) shutdownDispatch() {
	h.ctx.Lock.Lock()
	defer h.ctx.Lock.Unlock()

	startTime := time.Now()
	if err := h.engine.Shutdown(); err != nil {
		h.ctx.Log.Error("Error while shutting down the chain: %s", err)
	}
	if h.toClose != nil {
		go h.toClose()
	}
	h.closing.SetValue(true)
	h.metrics.shutdown.Observe(float64(time.Since(startTime)))
	close(h.closed)
}

func (h *Handler) handleValidatorMsg(msg message, startTime time.Time) error {
	var (
		err error
	)
	switch msg.messageType {
	case constants.GetAcceptedFrontierMsg:
		err = h.engine.GetAcceptedFrontier(msg.validatorID, msg.requestID)
	case constants.AcceptedFrontierMsg:
		err = h.engine.AcceptedFrontier(msg.validatorID, msg.requestID, msg.containerIDs)
	case constants.GetAcceptedFrontierFailedMsg:
		err = h.engine.GetAcceptedFrontierFailed(msg.validatorID, msg.requestID)
	case constants.GetAcceptedMsg:
		err = h.engine.GetAccepted(msg.validatorID, msg.requestID, msg.containerIDs)
	case constants.AcceptedMsg:
		err = h.engine.Accepted(msg.validatorID, msg.requestID, msg.containerIDs)
	case constants.GetAcceptedFailedMsg:
		err = h.engine.GetAcceptedFailed(msg.validatorID, msg.requestID)
	case constants.GetAncestorsMsg:
		err = h.engine.GetAncestors(msg.validatorID, msg.requestID, msg.containerID)
	case constants.GetAncestorsFailedMsg:
		err = h.engine.GetAncestorsFailed(msg.validatorID, msg.requestID)
	case constants.MultiPutMsg:
		err = h.engine.MultiPut(msg.validatorID, msg.requestID, msg.containers)
	case constants.GetMsg:
		err = h.engine.Get(msg.validatorID, msg.requestID, msg.containerID)
	case constants.GetFailedMsg:
		err = h.engine.GetFailed(msg.validatorID, msg.requestID)
	case constants.PutMsg:
		err = h.engine.Put(msg.validatorID, msg.requestID, msg.containerID, msg.container)
	case constants.PushQueryMsg:
		err = h.engine.PushQuery(msg.validatorID, msg.requestID, msg.containerID, msg.container)
	case constants.PullQueryMsg:
		err = h.engine.PullQuery(msg.validatorID, msg.requestID, msg.containerID)
	case constants.QueryFailedMsg:
		err = h.engine.QueryFailed(msg.validatorID, msg.requestID)
	case constants.ChitsMsg:
		err = h.engine.Chits(msg.validatorID, msg.requestID, msg.containerIDs)
	case constants.ConnectedMsg:
		err = h.engine.Connected(msg.validatorID)
	case constants.DisconnectedMsg:
		err = h.engine.Disconnected(msg.validatorID)
	}
	endTime := h.clock.Time()
	timeConsumed := endTime.Sub(startTime)

	histogram := h.metrics.getMSGHistogram(msg.messageType)
	histogram.Observe(float64(timeConsumed))

	h.cpuTracker.UtilizeTime(msg.validatorID, startTime, endTime)
	h.serviceQueue.UtilizeCPU(msg.validatorID, timeConsumed)
	return err
}

func (h *Handler) sendReliableMsg(msg message) {
	h.reliableMsgsLock.Lock()
	defer h.reliableMsgsLock.Unlock()

	h.metrics.pending.Inc()
	h.reliableMsgs = append(h.reliableMsgs, msg)
	select {
	case h.reliableMsgsSema <- struct{}{}:
	default:
	}
}

func (h *Handler) endInterval() {
	endTime := h.clock.Time()
	h.cpuTracker.EndInterval(endTime)
	h.serviceQueue.EndInterval(endTime)
}
