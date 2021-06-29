// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	cryptorand "crypto/rand"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/health"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/networking/benchlist"
	"github.com/ava-labs/avalanchego/snow/networking/router"
	"github.com/ava-labs/avalanchego/snow/networking/sender"
	"github.com/ava-labs/avalanchego/snow/triggers"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/sampler"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/version"
)

// reasonable default values
const (
	defaultInitialReconnectDelay                     = time.Second
	defaultMaxReconnectDelay                         = time.Hour
	DefaultMaxMessageSize                     uint32 = 2 * 1024 * 1024   // 2 MB
	defaultMaxNetworkPendingSendBytes                = 512 * 1024 * 1024 // 512 MB
	defaultNetworkPendingSendBytesToRateLimit        = defaultMaxNetworkPendingSendBytes / 4
	defaultMaxClockDifference                        = time.Minute
	defaultPeerListStakerGossipFraction              = 2
	defaultGetVersionTimeout                         = 10 * time.Second
	defaultAllowPrivateIPs                           = true
	defaultPingPongTimeout                           = 30 * time.Second
	defaultPingFrequency                             = 3 * defaultPingPongTimeout / 4
	defaultReadBufferSize                            = 16 * 1024 // 16 KB
	defaultReadHandshakeTimeout                      = 15 * time.Second
	defaultConnMeterCacheSize                        = 1024
	defaultByteSliceCap                              = 128
	defaultConcurrentUpgrades                        = 64
)

var (
	errNetworkClosed         = errors.New("network closed")
	errPeerIsMyself          = errors.New("peer is myself")
	errNetworkLayerUnhealthy = errors.New("network layer is unhealthy")
)

var _ Network = &network{}

func init() { rand.Seed(time.Now().UnixNano()) }

// Network defines the functionality of the networking library.
type Network interface {
	// All consensus messages can be sent through this interface. Thread safety
	// must be managed internally in the network.
	sender.ExternalSender

	// The network must be able to broadcast accepted decisions to random peers.
	// Thread safety must be managed internally in the network.
	triggers.Acceptor

	// Should only be called once, will run until either a fatal error occurs,
	// or the network is closed. Returns a non-nil error.
	Dispatch() error

	// Attempt to connect to this IP. Thread safety must be managed internally
	// to the network. The network will never stop attempting to connect to this
	// IP.
	TrackIP(ip utils.IPDesc)

	// Attempt to connect to this node ID at IP. Thread safety must be managed
	// internally to the network.
	Track(ip utils.IPDesc, nodeID ids.ShortID)

	// Returns the description of the specified [nodeIDs] this network is currently
	// connected to externally or all nodes this network is connected to if [nodeIDs]
	// is empty. Thread safety must be managed internally to the network.
	Peers(nodeIDs []ids.ShortID) []PeerID

	// Close this network and all existing connections it has. Thread safety
	// must be managed internally to the network. Calling close multiple times
	// will return a nil error.
	Close() error

	// Return the IP of the node
	IP() utils.IPDesc

	// Has a health check
	health.Checkable
}

type network struct {
	// The metrics that this network tracks
	metrics
	// Define the parameters used to determine whether
	// the networking layer is healthy
	healthConfig HealthConfig
	// Unix time at which last message of any type received over network
	// Must only be accessed atomically
	lastMsgReceivedTime int64
	// Unix time at which last message of any type sent over network
	// Must only be accessed atomically
	lastMsgSentTime int64
	// Keeps track of the percentage of sends that fail
	sendFailRateCalculator             math.Averager
	log                                logging.Logger
	id                                 ids.ShortID
	ip                                 utils.DynamicIPDesc
	networkID                          uint32
	versionCompatibility               version.Compatibility
	parser                             version.ApplicationParser
	listener                           net.Listener
	dialer                             Dialer
	serverUpgrader                     Upgrader
	clientUpgrader                     Upgrader
	vdrs                               validators.Set // set of current validators in the Avalanche network
	beacons                            validators.Set // set of beacons in the Avalanche network
	router                             router.Router  // router must be thread safe
	nodeID                             uint32
	clock                              timer.Clock
	initialReconnectDelay              time.Duration
	maxReconnectDelay                  time.Duration
	maxMessageSize                     int64
	sendQueueSize                      uint32
	maxNetworkPendingSendBytes         int64
	networkPendingSendBytesToRateLimit int64
	maxClockDifference                 time.Duration
	// Size of a peer list sent to peers
	peerListSize int
	// Gossip a peer list to peers with this frequency
	peerListGossipFreq time.Duration
	// Gossip a peer list to this many peers when gossiping
	peerListGossipSize           int
	peerListStakerGossipFraction int
	dialerConfig                 DialerConfig
	getVersionTimeout            time.Duration
	allowPrivateIPs              bool
	gossipAcceptedFrontierSize   uint
	gossipOnAcceptSize           uint
	pingPongTimeout              time.Duration
	pingFrequency                time.Duration
	readBufferSize               uint32
	readHandshakeTimeout         time.Duration
	connMeter                    ConnMeter
	b                            Builder
	isFetchOnly                  bool

	// stateLock should never be held when grabbing a peer senderLock
	stateLock    sync.RWMutex
	pendingBytes int64
	closed       utils.AtomicBool

	// May contain peers that we have not finished the handshake with.
	peers peersData

	// disconnectedIPs, connectedIPs, peerAliasIPs, and myIPs
	// are maps with ip.String() keys that are used to determine if
	// we should attempt to dial an IP. [stateLock] should be held
	// whenever accessing one of these maps.
	disconnectedIPs map[string]struct{} // set of IPs we are attempting to connect to
	connectedIPs    map[string]struct{} // set of IPs we have open connections with
	peerAliasIPs    map[string]struct{} // set of alternate IPs we've reached existing peers at
	// TODO: bound the size of [myIPs] to avoid DoS. LRU caching would be ideal
	myIPs map[string]struct{} // set of IPs that resulted in my ID.

	// retryDelay is a map with ip.String() keys that is used to track
	// the backoff delay we should wait before attempting to dial an IP address
	// again.
	retryDelay map[string]time.Duration

	// peerAliasTimeout is the age a peer alias must
	// be before we attempt to release it (so that we
	// attempt to dial the IP again if gossiped to us).
	peerAliasTimeout time.Duration

	// ensures the close of the network only happens once.
	closeOnce sync.Once

	hasMasked        bool
	maskedValidators ids.ShortSet

	benchlistManager benchlist.Manager

	// this node's TLS key
	tlsKey crypto.Signer

	// [lastTimestampLock] should be held when touching  [lastVersionIP],
	// [lastVersionTimestamp], and [lastVersionSignature]
	timeForIPLock sync.Mutex
	// The IP for ourself that we included in the most recent Version message we
	// sent.
	lastVersionIP utils.IPDesc
	// The timestamp we included in the most recent Version message we sent.
	lastVersionTimestamp uint64
	// The signature we included in the most recent Version message we sent.
	lastVersionSignature []byte

	// Node ID --> Latest IP/timestamp of this node from a Version or PeerList message
	// The values in this map all have [signature] == nil
	// A peer is removed from this map when [connected] is called with the peer as the argument
	// TODO also remove from this map when the peer leaves the validator set
	latestPeerIP map[ids.ShortID]signedPeerIP

	// Node ID --> Function to execute to stop trying to dial the node.
	// A node is present in this map if and only if we are actively
	// trying to dial the node.
	connAttempts sync.Map

	upgradeSem chan struct{}

	// Contains []byte. Used as an optimization.
	// Can be accessed by multiple goroutines concurrently.
	byteSlicePool sync.Pool
}

// NewDefaultNetwork returns a new Network implementation with the provided
// parameters and some reasonable default values.
func NewDefaultNetwork(
	registerer prometheus.Registerer,
	log logging.Logger,
	id ids.ShortID,
	ip utils.DynamicIPDesc,
	networkID uint32,
	versionCompatibility version.Compatibility,
	parser version.ApplicationParser,
	listener net.Listener,
	dialer Dialer,
	serverUpgrader,
	clientUpgrader Upgrader,
	vdrs validators.Set,
	beacons validators.Set,
	router router.Router,
	connMeterResetDuration time.Duration,
	connMeterMaxConns int,
	sendQueueSize uint32,
	healthConfig HealthConfig,
	benchlistManager benchlist.Manager,
	peerAliasTimeout time.Duration,
	tlsKey crypto.Signer,
	peerListSize int,
	peerListGossipSize int,
	peerListGossipFreq time.Duration,
	dialerConfig DialerConfig,
	isFetchOnly bool,
	gossipAcceptedFrontierSize uint,
	gossipOnAcceptSize uint,
) Network {
	return NewNetwork(
		registerer,
		log,
		id,
		ip,
		networkID,
		versionCompatibility,
		parser,
		listener,
		dialer,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		beacons,
		router,
		defaultInitialReconnectDelay,
		defaultMaxReconnectDelay,
		DefaultMaxMessageSize,
		sendQueueSize,
		defaultMaxNetworkPendingSendBytes,
		defaultNetworkPendingSendBytesToRateLimit,
		defaultMaxClockDifference,
		peerListSize,
		peerListGossipFreq,
		peerListGossipSize,
		defaultPeerListStakerGossipFraction,
		defaultGetVersionTimeout,
		defaultAllowPrivateIPs,
		gossipAcceptedFrontierSize,
		gossipOnAcceptSize,
		defaultPingPongTimeout,
		defaultPingFrequency,
		defaultReadBufferSize,
		defaultReadHandshakeTimeout,
		connMeterResetDuration,
		defaultConnMeterCacheSize,
		connMeterMaxConns,
		healthConfig,
		benchlistManager,
		peerAliasTimeout,
		dialerConfig,
		tlsKey,
		isFetchOnly,
	)
}

// NewNetwork returns a new Network implementation with the provided parameters.
func NewNetwork(
	registerer prometheus.Registerer,
	log logging.Logger,
	id ids.ShortID,
	ip utils.DynamicIPDesc,
	networkID uint32,
	versionCompatibility version.Compatibility,
	parser version.ApplicationParser,
	listener net.Listener,
	dialer Dialer,
	serverUpgrader,
	clientUpgrader Upgrader,
	vdrs validators.Set,
	beacons validators.Set,
	router router.Router,
	initialReconnectDelay,
	maxReconnectDelay time.Duration,
	maxMessageSize uint32,
	sendQueueSize uint32,
	maxNetworkPendingSendBytes int,
	networkPendingSendBytesToRateLimit int,
	maxClockDifference time.Duration,
	peerListSize int,
	peerListGossipFreq time.Duration,
	peerListGossipSize int,
	peerListStakerGossipFraction int,
	getVersionTimeout time.Duration,
	allowPrivateIPs bool,
	gossipAcceptedFrontierSize uint,
	gossipOnAcceptSize uint,
	pingPongTimeout time.Duration,
	pingFrequency time.Duration,
	readBufferSize uint32,
	readHandshakeTimeout time.Duration,
	connMeterResetDuration time.Duration,
	connMeterCacheSize int,
	connMeterMaxConns int,
	healthConfig HealthConfig,
	benchlistManager benchlist.Manager,
	peerAliasTimeout time.Duration,
	dialerConfig DialerConfig,
	tlsKey crypto.Signer,
	isFetchOnly bool,
) Network {
	// #nosec G404
	netw := &network{
		log:                  log,
		id:                   id,
		ip:                   ip,
		networkID:            networkID,
		versionCompatibility: versionCompatibility,
		parser:               parser,
		listener:             listener,
		dialer:               dialer,
		serverUpgrader:       serverUpgrader,
		clientUpgrader:       clientUpgrader,
		vdrs:                 vdrs,
		beacons:              beacons,
		router:               router,
		// This field just makes sure we don't connect to ourselves when TLS is
		// disabled. So, cryptographically secure random number generation isn't
		// used here.
		nodeID:                             rand.Uint32(),
		initialReconnectDelay:              initialReconnectDelay,
		maxReconnectDelay:                  maxReconnectDelay,
		maxMessageSize:                     int64(maxMessageSize),
		sendQueueSize:                      sendQueueSize,
		maxNetworkPendingSendBytes:         int64(maxNetworkPendingSendBytes),
		networkPendingSendBytesToRateLimit: int64(networkPendingSendBytesToRateLimit),
		maxClockDifference:                 maxClockDifference,
		peerListSize:                       peerListSize,
		peerListGossipFreq:                 peerListGossipFreq,
		peerListGossipSize:                 peerListGossipSize,
		peerListStakerGossipFraction:       peerListStakerGossipFraction,
		dialerConfig:                       dialerConfig,
		getVersionTimeout:                  getVersionTimeout,
		allowPrivateIPs:                    allowPrivateIPs,
		gossipAcceptedFrontierSize:         gossipAcceptedFrontierSize,
		gossipOnAcceptSize:                 gossipOnAcceptSize,
		pingPongTimeout:                    pingPongTimeout,
		pingFrequency:                      pingFrequency,
		disconnectedIPs:                    make(map[string]struct{}),
		connectedIPs:                       make(map[string]struct{}),
		peerAliasIPs:                       make(map[string]struct{}),
		peerAliasTimeout:                   peerAliasTimeout,
		retryDelay:                         make(map[string]time.Duration),
		myIPs:                              map[string]struct{}{ip.IP().String(): {}},
		readBufferSize:                     readBufferSize,
		readHandshakeTimeout:               readHandshakeTimeout,
		connMeter:                          NewConnMeter(connMeterResetDuration, connMeterCacheSize, connMeterMaxConns),
		healthConfig:                       healthConfig,
		benchlistManager:                   benchlistManager,
		tlsKey:                             tlsKey,
		latestPeerIP:                       make(map[ids.ShortID]signedPeerIP),
		isFetchOnly:                        isFetchOnly,
		byteSlicePool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, defaultByteSliceCap)
			},
		},
		upgradeSem: make(chan struct{}, defaultConcurrentUpgrades),
	}
	netw.b = Builder{
		getByteSlice: func() []byte {
			return netw.byteSlicePool.Get().([]byte)
		},
	}
	netw.peers.initialize()
	netw.sendFailRateCalculator = math.NewSyncAverager(math.NewAverager(0, healthConfig.MaxSendFailRateHalflife, netw.clock.Time()))

	if err := netw.initialize(registerer); err != nil {
		log.Warn("initializing network metrics failed with: %s", err)
	}
	return netw
}

// GetAcceptedFrontier implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) GetAcceptedFrontier(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration) []ids.ShortID {
	msg, err := n.b.GetAcceptedFrontier(chainID, requestID, uint64(deadline))
	n.log.AssertNoError(err)

	sentTo := make([]ids.ShortID, 0, validatorIDs.Len())
	now := n.clock.Time()
	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		lenMsg := len(msg.Bytes())
		if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, false) {
			n.log.Debug("failed to send GetAcceptedFrontier(%s, %s, %d)",
				vID,
				chainID,
				requestID)
			n.getAcceptedFrontier.numFailed.Inc()
			n.sendFailRateCalculator.Observe(1, now)
		} else {
			sentTo = append(sentTo, vID)
			n.getAcceptedFrontier.numSent.Inc()
			n.sendFailRateCalculator.Observe(0, now)
			n.getAcceptedFrontier.sentBytes.Add(float64(lenMsg))
		}
	}
	return sentTo
}

// AcceptedFrontier implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) AcceptedFrontier(nodeID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID) {
	now := n.clock.Time()

	msg, err := n.b.AcceptedFrontier(chainID, requestID, containerIDs)
	if err != nil {
		n.log.Error("failed to build AcceptedFrontier(%s, %d, %s): %s",
			chainID,
			requestID,
			containerIDs,
			err)
		n.sendFailRateCalculator.Observe(1, now)
		return // Packing message failed
	}

	peer := n.getPeer(nodeID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send AcceptedFrontier(%s, %s, %d, %s)",
			nodeID,
			chainID,
			requestID,
			containerIDs)
		n.acceptedFrontier.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
	} else {
		n.acceptedFrontier.numSent.Inc()
		n.sendFailRateCalculator.Observe(0, now)
		n.acceptedFrontier.sentBytes.Add(float64(lenMsg))
	}
}

// GetAccepted implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) GetAccepted(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerIDs []ids.ID) []ids.ShortID {
	now := n.clock.Time()

	msg, err := n.b.GetAccepted(chainID, requestID, uint64(deadline), containerIDs)
	if err != nil {
		n.log.Error("failed to build GetAccepted(%s, %d, %s): %s",
			chainID,
			requestID,
			containerIDs,
			err)
		n.sendFailRateCalculator.Observe(1, now)
		return nil
	}

	sentTo := make([]ids.ShortID, 0, validatorIDs.Len())
	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		lenMsg := len(msg.Bytes())
		if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, false) {
			n.log.Debug("failed to send GetAccepted(%s, %s, %d, %s)",
				vID,
				chainID,
				requestID,
				containerIDs)
			n.getAccepted.numFailed.Inc()
			n.sendFailRateCalculator.Observe(1, now)
		} else {
			n.getAccepted.numSent.Inc()
			n.sendFailRateCalculator.Observe(0, now)
			n.getAccepted.sentBytes.Add(float64(lenMsg))
			sentTo = append(sentTo, vID)
		}
	}
	return sentTo
}

// Accepted implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) Accepted(nodeID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID) {
	now := n.clock.Time()

	msg, err := n.b.Accepted(chainID, requestID, containerIDs)
	if err != nil {
		n.log.Error("failed to build Accepted(%s, %d, %s): %s",
			chainID,
			requestID,
			containerIDs,
			err)
		n.sendFailRateCalculator.Observe(1, now)
		return // Packing message failed
	}

	peer := n.getPeer(nodeID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send Accepted(%s, %s, %d, %s)",
			nodeID,
			chainID,
			requestID,
			containerIDs)
		n.accepted.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
	} else {
		n.sendFailRateCalculator.Observe(0, now)
		n.accepted.numSent.Inc()
		n.accepted.sentBytes.Add(float64(lenMsg))
	}
}

// GetAncestors implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) GetAncestors(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) bool {
	now := n.clock.Time()

	msg, err := n.b.GetAncestors(chainID, requestID, uint64(deadline), containerID)
	if err != nil {
		n.log.Error("failed to build GetAncestors message: %s", err)
		n.sendFailRateCalculator.Observe(1, now)
		return false
	}

	peer := n.getPeer(validatorID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send GetAncestors(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			containerID)
		n.getAncestors.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
		return false
	}
	n.getAncestors.numSent.Inc()
	n.sendFailRateCalculator.Observe(0, now)
	n.getAncestors.sentBytes.Add(float64(lenMsg))
	return true
}

// MultiPut implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) MultiPut(nodeID ids.ShortID, chainID ids.ID, requestID uint32, containers [][]byte) {
	now := n.clock.Time()

	msg, err := n.b.MultiPut(chainID, requestID, containers)
	if err != nil {
		n.log.Error("failed to build MultiPut message because of container of size %d", len(containers))
		n.sendFailRateCalculator.Observe(1, now)
		return
	}

	peer := n.getPeer(nodeID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send MultiPut(%s, %s, %d, %d)",
			nodeID,
			chainID,
			requestID,
			len(containers))
		n.multiPut.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
	} else {
		n.multiPut.numSent.Inc()
		n.sendFailRateCalculator.Observe(0, now)
		n.multiPut.sentBytes.Add(float64(lenMsg))
	}
}

// Get implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) Get(nodeID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) bool {
	now := n.clock.Time()

	msg, err := n.b.Get(chainID, requestID, uint64(deadline), containerID)
	n.log.AssertNoError(err)

	peer := n.getPeer(nodeID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send Get(%s, %s, %d, %s)",
			nodeID,
			chainID,
			requestID,
			containerID)
		n.get.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
		return false
	}
	n.get.numSent.Inc()
	n.sendFailRateCalculator.Observe(0, now)
	n.get.sentBytes.Add(float64(lenMsg))
	return true
}

// Put implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) Put(nodeID ids.ShortID, chainID ids.ID, requestID uint32, containerID ids.ID, container []byte) {
	now := n.clock.Time()

	msg, err := n.b.Put(chainID, requestID, containerID, container)
	if err != nil {
		n.log.Error("failed to build Put(%s, %d, %s): %s. len(container) : %d",
			chainID,
			requestID,
			containerID,
			err,
			len(container))
		n.sendFailRateCalculator.Observe(1, now)
		return
	}

	peer := n.getPeer(nodeID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send Put(%s, %s, %d, %s)",
			nodeID,
			chainID,
			requestID,
			containerID)
		n.log.Verbo("container: %s", formatting.DumpBytes{Bytes: container})
		n.put.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
	} else {
		n.put.numSent.Inc()
		n.sendFailRateCalculator.Observe(0, now)
		n.put.sentBytes.Add(float64(lenMsg))
	}
}

// PushQuery implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) PushQuery(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID, container []byte) []ids.ShortID {
	now := n.clock.Time()

	msg, err := n.b.PushQuery(chainID, requestID, uint64(deadline), containerID, container)
	if err != nil {
		n.log.Error("failed to build PushQuery(%s, %d, %s): %s. len(container): %d",
			chainID,
			requestID,
			containerID,
			err,
			len(container))
		n.log.Verbo("container: %s", formatting.DumpBytes{Bytes: container})
		n.sendFailRateCalculator.Observe(1, now)
		return nil // Packing message failed
	}

	sentTo := make([]ids.ShortID, 0, validatorIDs.Len())
	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		lenMsg := len(msg.Bytes())
		if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, false) {
			n.log.Debug("failed to send PushQuery(%s, %s, %d, %s)",
				vID,
				chainID,
				requestID,
				containerID)
			n.log.Verbo("container: %s", formatting.DumpBytes{Bytes: container})
			n.pushQuery.numFailed.Inc()
			n.sendFailRateCalculator.Observe(1, now)
		} else {
			n.pushQuery.numSent.Inc()
			sentTo = append(sentTo, vID)
			n.sendFailRateCalculator.Observe(0, now)
			n.pushQuery.sentBytes.Add(float64(lenMsg))
		}
	}
	return sentTo
}

// PullQuery implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) PullQuery(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Duration, containerID ids.ID) []ids.ShortID {
	now := n.clock.Time()

	msg, err := n.b.PullQuery(chainID, requestID, uint64(deadline), containerID)
	n.log.AssertNoError(err)

	sentTo := make([]ids.ShortID, 0, validatorIDs.Len())
	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		lenMsg := len(msg.Bytes())
		if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, false) {
			n.log.Debug("failed to send PullQuery(%s, %s, %d, %s)",
				vID,
				chainID,
				requestID,
				containerID)
			n.pullQuery.numFailed.Inc()
			n.sendFailRateCalculator.Observe(1, now)
		} else {
			n.pullQuery.numSent.Inc()
			n.sendFailRateCalculator.Observe(0, now)
			sentTo = append(sentTo, vID)
			n.pullQuery.sentBytes.Add(float64(lenMsg))
		}
	}
	return sentTo
}

// Chits implements the Sender interface.
// Assumes [n.stateLock] is not held.
func (n *network) Chits(nodeID ids.ShortID, chainID ids.ID, requestID uint32, votes []ids.ID) {
	now := n.clock.Time()

	msg, err := n.b.Chits(chainID, requestID, votes)
	if err != nil {
		n.log.Error("failed to build Chits(%s, %d, %s): %s",
			chainID,
			requestID,
			votes,
			err)
		n.sendFailRateCalculator.Observe(1, now)
		return
	}

	peer := n.getPeer(nodeID)
	lenMsg := len(msg.Bytes())
	if peer == nil || !peer.finishedHandshake.GetValue() || !peer.Send(msg, true) {
		n.log.Debug("failed to send Chits(%s, %s, %d, %s)",
			nodeID,
			chainID,
			requestID,
			votes)
		n.chits.numFailed.Inc()
		n.sendFailRateCalculator.Observe(1, now)
	} else {
		n.sendFailRateCalculator.Observe(0, now)
		n.chits.numSent.Inc()
		n.chits.sentBytes.Add(float64(lenMsg))
	}
}

// Gossip attempts to gossip the container to the network
// Assumes [n.stateLock] is not held.
func (n *network) Gossip(chainID, containerID ids.ID, container []byte) {
	if err := n.gossipContainer(chainID, containerID, container, n.gossipAcceptedFrontierSize); err != nil {
		n.log.Debug("failed to Gossip(%s, %s): %s", chainID, containerID, err)
		n.log.Verbo("container:\n%s", formatting.DumpBytes{Bytes: container})
	}
}

// Accept is called after every consensus decision
// Assumes [n.stateLock] is not held.
func (n *network) Accept(ctx *snow.Context, containerID ids.ID, container []byte) error {
	if !ctx.IsBootstrapped() {
		// don't gossip during bootstrapping
		return nil
	}
	return n.gossipContainer(ctx.ChainID, containerID, container, n.gossipOnAcceptSize)
}

// shouldUpgradeIncoming returns whether we should
// upgrade an incoming connection from a peer
// at the IP whose string repr. is [ipStr].
// Assumes stateLock is not held.
func (n *network) shouldUpgradeIncoming(ipStr string) bool {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if _, ok := n.connectedIPs[ipStr]; ok {
		n.log.Debug("not upgrading connection to %s because it's connected", ipStr)
		return false
	}
	if _, ok := n.myIPs[ipStr]; ok {
		n.log.Debug("not upgrading connection to %s because it's myself", ipStr)
		return false
	}
	if _, ok := n.peerAliasIPs[ipStr]; ok {
		n.log.Debug("not upgrading connection to %s because it's an alias", ipStr)
		return false
	}
	if !n.connMeter.Allow(ipStr) {
		n.log.Debug("not upgrading connection to %s due to rate-limiting", ipStr)
		return false
	}

	// Note that we attempt to upgrade remote addresses in
	// [n.disconnectedIPs] because that could allow us to initialize
	// a connection with a peer we've been attempting to dial.
	return true
}

// Dispatch starts accepting connections from other nodes attempting to connect
// to this node.
// Assumes [n.stateLock] is not held.
func (n *network) Dispatch() error {
	go n.gossipPeerList() // Periodically gossip peers
	go func() {
		duration := time.Until(n.versionCompatibility.MaskTime())
		time.Sleep(duration)

		n.stateLock.Lock()
		defer n.stateLock.Unlock()

		n.hasMasked = true
		for _, vdrID := range n.maskedValidators.List() {
			if err := n.vdrs.MaskValidator(vdrID); err != nil {
				n.log.Error("failed to mask validator %s due to %s", vdrID, err)
			}
		}
		n.maskedValidators.Clear()
		n.log.Verbo("The new staking set is:\n%s", n.vdrs)
	}()

	for { // Continuously accept new connections
		conn, err := n.listener.Accept() // Returns error when n.Close() is called
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				// Sleep for a small amount of time to try to wait for the
				// temporary error to go away.
				time.Sleep(time.Millisecond)
				continue
			}

			// When [n].Close() is called, [n.listener].Close() is called.
			// This causes [n.listener].Accept() to return an error.
			// If that happened, don't log/return an error here.
			if n.closed.GetValue() {
				return errNetworkClosed
			}
			n.log.Debug("error during server accept: %s", err)
			return err
		}

		// We pessimistically drop an incoming connection if the remote
		// address is found in connectedIPs, myIPs, or peerAliasIPs.
		// This protects our node from spending CPU cycles on TLS
		// handshakes to upgrade connections from existing peers.
		// Specifically, this can occur when one of our existing
		// peers attempts to connect to one our IP aliases (that they
		// aren't yet aware is an alias).
		remoteAddr := conn.RemoteAddr().String()
		ip, err := utils.ToIPDesc(remoteAddr)
		if err != nil {
			return fmt.Errorf("unable to convert remote address %s to IPDesc: %w", remoteAddr, err)
		}
		ipStr := ip.IP.String()
		if !n.shouldUpgradeIncoming(ipStr) {
			_ = conn.Close()
			continue
		}
		select {
		case n.upgradeSem <- struct{}{}:
		default:
			n.log.Debug("discarding incoming conn due to too many concurrent upgrades: %s", ipStr)
			_ = conn.Close()
			continue
		}

		if conn, ok := conn.(*net.TCPConn); ok {
			if err := conn.SetLinger(0); err != nil {
				n.log.Warn("failed to set no linger due to: %s", err)
			}
			if err := conn.SetNoDelay(true); err != nil {
				n.log.Warn("failed to set socket nodelay due to: %s", err)
			}
		}

		go func() {
			if err := n.upgrade(newPeer(n, conn, utils.IPDesc{}), n.serverUpgrader); err != nil {
				n.log.Debug("failed to upgrade incoming connection: %s", err)
			}
			<-n.upgradeSem
		}()
	}
}

// Returns information about peers.
// If [nodeIDs] is empty, returns info about all peers that have finished
// the handshake. Otherwise, returns info about the peers in [nodeIDs]
// that have finished the handshake.
// Assumes [n.stateLock] is not held.
func (n *network) Peers(nodeIDs []ids.ShortID) []PeerID {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if len(nodeIDs) == 0 { // Return info about all peers
		peers := make([]PeerID, 0, n.peers.size())
		for _, peer := range n.peers.peersList {
			if peer.finishedHandshake.GetValue() {
				peers = append(peers, PeerID{
					IP:           peer.conn.RemoteAddr().String(),
					PublicIP:     peer.getIP().String(),
					ID:           peer.nodeID.PrefixedString(constants.NodeIDPrefix),
					Version:      peer.versionStr.GetValue().(string),
					LastSent:     time.Unix(atomic.LoadInt64(&peer.lastSent), 0),
					LastReceived: time.Unix(atomic.LoadInt64(&peer.lastReceived), 0),
					Benched:      n.benchlistManager.GetBenched(peer.nodeID),
				})
			}
		}
		return peers
	}

	peers := make([]PeerID, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs { // Return info about given peers
		if peer, ok := n.peers.getByID(nodeID); ok && peer.finishedHandshake.GetValue() {
			peers = append(peers, PeerID{
				IP:           peer.conn.RemoteAddr().String(),
				PublicIP:     peer.getIP().String(),
				ID:           peer.nodeID.PrefixedString(constants.NodeIDPrefix),
				Version:      peer.versionStr.GetValue().(string),
				LastSent:     time.Unix(atomic.LoadInt64(&peer.lastSent), 0),
				LastReceived: time.Unix(atomic.LoadInt64(&peer.lastReceived), 0),
				Benched:      n.benchlistManager.GetBenched(peer.nodeID),
			})
		}
	}
	return peers
}

// Close implements the Network interface
// Assumes [n.stateLock] is not held.
func (n *network) Close() error {
	n.closeOnce.Do(n.close)
	return nil
}

func (n *network) close() {
	n.log.Info("shutting down network")

	if err := n.listener.Close(); err != nil {
		n.log.Debug("closing network listener failed with: %s", err)
	}

	if n.closed.GetValue() {
		return
	}

	n.stateLock.Lock()
	if n.closed.GetValue() {
		n.stateLock.Unlock()
		return
	}
	n.closed.SetValue(true)

	peersToClose := make([]*peer, n.peers.size())
	copy(peersToClose, n.peers.peersList)
	n.peers.reset()
	n.stateLock.Unlock()

	for _, peer := range peersToClose {
		peer.Close() // Grabs the stateLock
	}
}

// TrackIP implements the Network interface
// Assumes [n.stateLock] is not held.
func (n *network) TrackIP(ip utils.IPDesc) {
	n.Track(ip, ids.ShortEmpty)
}

// Track implements the Network interface
// Assumes [n.stateLock] is not held.
func (n *network) Track(ip utils.IPDesc, nodeID ids.ShortID) {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	n.track(ip, nodeID)
}

func (n *network) IP() utils.IPDesc {
	return n.ip.IP()
}

// Assumes [n.stateLock] is not held.
func (n *network) gossipContainer(chainID, containerID ids.ID, container []byte, numToGossip uint) error {
	now := n.clock.Time()

	msg, err := n.b.Put(chainID, constants.GossipMsgRequestID, containerID, container)
	if err != nil {
		n.sendFailRateCalculator.Observe(1, now)
		return fmt.Errorf("attempted to pack too large of a Put message.\nContainer length: %d", len(container))
	}

	allPeers := n.getAllPeers()

	if int(numToGossip) > len(allPeers) {
		numToGossip = uint(len(allPeers))
	}

	s := sampler.NewUniform()
	if err := s.Initialize(uint64(len(allPeers))); err != nil {
		return err
	}
	indices, err := s.Sample(int(numToGossip))
	if err != nil {
		return err
	}
	for _, index := range indices {
		if allPeers[int(index)].Send(msg, false) {
			n.put.numSent.Inc()
			n.sendFailRateCalculator.Observe(0, now)
		} else {
			n.sendFailRateCalculator.Observe(1, now)
			n.put.numFailed.Inc()
		}
	}
	return nil
}

// assumes the stateLock is held.
// Try to connect to [nodeID] at [ip].
func (n *network) track(ip utils.IPDesc, nodeID ids.ShortID) {
	if n.closed.GetValue() {
		return
	}

	str := ip.String()
	if _, ok := n.disconnectedIPs[str]; ok {
		return
	}
	if _, ok := n.connectedIPs[str]; ok {
		return
	}
	if _, ok := n.peerAliasIPs[str]; ok {
		return
	}
	if _, ok := n.myIPs[str]; ok {
		return
	}
	// If we saw an IP gossiped for this node ID
	// with a later timestamp, don't track this old IP
	if latestIP, ok := n.latestPeerIP[nodeID]; ok {
		if !latestIP.ip.Equal(ip) {
			return
		}
	}
	n.disconnectedIPs[str] = struct{}{}

	go n.connectTo(ip, nodeID)
}

// Assumes [n.stateLock] is not held. Only returns after the network is closed.
func (n *network) gossipPeerList() {
	t := time.NewTicker(n.peerListGossipFreq)
	defer t.Stop()

	for range t.C {
		if n.closed.GetValue() {
			return
		}

		allPeers := n.getAllPeers()
		if len(allPeers) == 0 {
			continue
		}

		stakers := make([]*peer, 0, len(allPeers))
		nonStakers := make([]*peer, 0, len(allPeers))
		for _, peer := range allPeers {
			if n.vdrs.Contains(peer.nodeID) {
				stakers = append(stakers, peer)
			} else {
				nonStakers = append(nonStakers, peer)
			}
		}

		numStakersToSend := (n.peerListGossipSize + n.peerListStakerGossipFraction - 1) / n.peerListStakerGossipFraction
		if len(stakers) < numStakersToSend {
			numStakersToSend = len(stakers)
		}
		numNonStakersToSend := n.peerListGossipSize - numStakersToSend
		if len(nonStakers) < numNonStakersToSend {
			numNonStakersToSend = len(nonStakers)
		}

		s := sampler.NewUniform()
		if err := s.Initialize(uint64(len(stakers))); err != nil {
			n.log.Error("failed to select stakers to sample: %s. len(stakers): %d",
				err,
				len(stakers))
			continue
		}
		stakerIndices, err := s.Sample(numStakersToSend)
		if err != nil {
			n.log.Error("failed to select stakers to sample: %s. len(stakers): %d",
				err,
				len(stakers))
			continue
		}

		if err := s.Initialize(uint64(len(nonStakers))); err != nil {
			n.log.Error("failed to select non-stakers to sample: %s. len(nonStakers): %d",
				err,
				len(nonStakers))
			continue
		}
		nonStakerIndices, err := s.Sample(numNonStakersToSend)
		if err != nil {
			n.log.Error("failed to select non-stakers to sample: %s. len(nonStakers): %d",
				err,
				len(nonStakers))
			continue
		}

		ipCerts, err := n.validatorIPs()
		if err != nil {
			n.log.Error("failed to fetch validator IPs: %s", err)
			continue
		}

		if len(ipCerts) == 0 {
			n.log.Debug("skipping validator IP gossiping as no IPs are connected")
			continue
		}

		msg, err := n.b.PeerList(ipCerts)
		if err != nil {
			n.log.Error("failed to build signed peerlist to gossip: %s. len(ips): %d",
				err,
				len(ipCerts))
			continue
		}

		for _, index := range stakerIndices {
			stakers[int(index)].Send(msg, false)
		}
		for _, index := range nonStakerIndices {
			nonStakers[int(index)].Send(msg, false)
		}
	}
}

// Returns when:
// * We connected to [ip]
// * The network is closed
// * We gave up connecting to [ip] because the IP is stale
// If [nodeID] == ids.ShortEmpty, won't cancel an existing
// attempt to connect to the peer with that IP.
// We do this so we don't cancel attempted to connect to bootstrap beacons.
// See method TrackIP.
// Assumes [n.stateLock] isn't held when this method is called.
func (n *network) connectTo(ip utils.IPDesc, nodeID ids.ShortID) {
	str := ip.String()
	n.stateLock.RLock()
	delay := n.retryDelay[str]
	n.stateLock.RUnlock()

	for {
		time.Sleep(delay)

		if delay == 0 {
			delay = n.initialReconnectDelay
		}

		// Randomization is only performed here to distribute reconnection
		// attempts to a node that previously shut down. This doesn't require
		// cryptographically secure random number generation.
		delay = time.Duration(float64(delay) * (1 + rand.Float64())) // #nosec G404
		if delay > n.maxReconnectDelay {
			// set the timeout to [.75, 1) * maxReconnectDelay
			delay = time.Duration(float64(n.maxReconnectDelay) * (3 + rand.Float64()) / 4) // #nosec G404
		}

		n.stateLock.Lock()
		_, isDisconnected := n.disconnectedIPs[str]
		_, isConnected := n.connectedIPs[str]
		_, isMyself := n.myIPs[str]
		// If we saw an IP gossiped for this node ID
		// with a later timestamp, don't track this old IP
		isLatestIP := true
		if latestIP, ok := n.latestPeerIP[nodeID]; ok {
			isLatestIP = latestIP.ip.Equal(ip)
		}
		closed := n.closed

		if !isDisconnected || !isLatestIP || isConnected || isMyself || closed.GetValue() {
			// If the IP was discovered by the peer connecting to us, we don't
			// need to attempt to connect anymore

			// If we've seen an IP gossiped for this peer with a later timestamp,
			// don't try to connect to the old IP anymore

			// If the IP was discovered to be our IP address, we don't need to
			// attempt to connect anymore

			// If the network was closed, we should stop attempting to connect
			// to the peer
			n.stateLock.Unlock()
			return
		}
		n.retryDelay[str] = delay
		n.stateLock.Unlock()

		// If we are already trying to connect to this node ID,
		// cancel the existing attempt.
		// If [nodeID] is the empty ID, [ip] is a bootstrap beacon.
		// In that case, don't cancel existing connection attempt.
		if nodeID != ids.ShortEmpty {
			if cancel, exists := n.connAttempts.Load(nodeID); exists {
				n.log.Verbo("canceling attempt to connect to stale IP of %s%s", constants.NodeIDPrefix, nodeID)
				cancel.(context.CancelFunc)()
			}
		}

		select {
		case n.upgradeSem <- struct{}{}:
		default:
			n.log.Debug("waiting on connection attempt due to too many concurrent upgrades: %s", ip.IP.String())
			continue
		}

		// When [cancel] is called, we give up on this attempt to connect
		// (if we have not yet connected.)
		// This occurs at the sooner of:
		// * [connectTo] is called again with the same node ID
		// * The call to [attemptConnect] below returns
		ctx, cancel := context.WithCancel(context.Background())
		n.connAttempts.Store(nodeID, cancel)

		// Attempt to connect
		err := n.attemptConnect(ctx, ip)
		<-n.upgradeSem
		cancel() // to avoid goroutine leak

		n.connAttempts.Delete(nodeID)

		if err == nil {
			return
		}
		n.log.Debug("error attempting to connect to %s: %s. Reattempting in %s",
			ip, err, delay)
	}
}

// Attempt to connect to the peer at [ip].
// If [ctx] is canceled, stops trying to connect.
// Returns nil if:
// * A connection was established
// * The network is closed.
// * [ctx] is canceled.
// Assumes [n.stateLock] is not held when this method is called.
func (n *network) attemptConnect(ctx context.Context, ip utils.IPDesc) error {
	n.log.Verbo("attempting to connect to %s", ip)
	conn, err := n.dialer.Dial(ctx, ip)
	if err != nil {
		// If [ctx] was canceled, return nil so we don't try to connect again
		if ctx.Err() == context.Canceled {
			return nil
		}
		// Error wasn't because connection attempt was canceled
		return err
	}

	if conn, ok := conn.(*net.TCPConn); ok {
		if err := conn.SetLinger(0); err != nil {
			n.log.Warn("failed to set no linger due to: %s", err)
		}
		if err := conn.SetNoDelay(true); err != nil {
			n.log.Warn("failed to set socket nodelay due to: %s", err)
		}
	}
	return n.upgrade(newPeer(n, conn, ip), n.clientUpgrader)
}

// Assumes [n.stateLock] is not held. Returns an error if the peer's connection
// wasn't able to be upgraded.
func (n *network) upgrade(p *peer, upgrader Upgrader) error {
	if err := p.conn.SetReadDeadline(time.Now().Add(n.readHandshakeTimeout)); err != nil {
		_ = p.conn.Close()
		n.log.Verbo("failed to set the read deadline with %s", err)
		return err
	}

	nodeID, conn, cert, err := upgrader.Upgrade(p.conn)
	if err != nil {
		_ = p.conn.Close()
		n.log.Debug("failed to upgrade connection with %s", err)
		return err
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		n.log.Verbo("failed to clear the read deadline with %s", err)
		return err
	}

	p.cert = cert
	p.sender = make(chan []byte, n.sendQueueSize)
	p.nodeID = nodeID
	p.conn = conn

	if err := n.tryAddPeer(p); err != nil {
		_ = p.conn.Close()
		n.log.Debug("dropping peer connection due to: %s", err)
	}
	return nil
}

// Returns an error if the peer couldn't be added.
// Assumes [n.stateLock] is not held.
func (n *network) tryAddPeer(p *peer) error {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	if n.closed.GetValue() {
		// the network is closing, so make sure that no further reconnect
		// attempts are made.
		return errNetworkClosed
	}

	ip := p.getIP()

	// if this connection is myself, then I should delete the connection and
	// mark the IP as one of mine.
	if p.nodeID == n.id {
		if !ip.IsZero() {
			// if n.ip is less useful than p.ip set it to this IP
			if n.ip.IP().IsZero() {
				n.log.Info("setting my ip to %s because I was able to connect to myself through this channel",
					p.ip)
				n.ip.Update(p.ip)
			}
			str := ip.String()
			delete(n.disconnectedIPs, str)
			delete(n.retryDelay, str)
			n.myIPs[str] = struct{}{}
		}
		return errPeerIsMyself
	}

	// If I am already connected to this peer, then I should close this new
	// connection and add an alias record.
	if peer, ok := n.peers.getByID(p.nodeID); ok {
		if !ip.IsZero() {
			str := ip.String()
			delete(n.disconnectedIPs, str)
			delete(n.retryDelay, str)
			peer.addAlias(ip)
		}
		return fmt.Errorf("duplicated connection from %s at %s", p.nodeID.PrefixedString(constants.NodeIDPrefix), ip)
	}

	n.peers.add(p)
	n.numPeers.Set(float64(n.peers.size()))
	p.Start()
	return nil
}

// Returns the IPs, certs and signatures of validators we're connected
// to that have finished the handshake.
// Assumes [n.stateLock] is not held.
func (n *network) validatorIPs() ([]utils.IPCertDesc, error) {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	totalNumPeers := n.peers.size()

	numToSend := n.peerListSize
	if totalNumPeers < numToSend {
		numToSend = totalNumPeers
	}

	if numToSend == 0 {
		return nil, nil
	}
	res := make([]utils.IPCertDesc, 0, numToSend)

	s := sampler.NewUniform()
	if err := s.Initialize(uint64(totalNumPeers)); err != nil {
		return nil, err
	}

	for len(res) != numToSend {
		sampledIdx, err := s.Next() // lazy-sampling
		if err != nil {
			// all peers have been sampled and not enough valid ones found.
			// return what we have
			return res, nil
		}

		// TODO: consider possibility of grouping peers in different buckets
		// (e.g. validators/non-validators, connected/disconnected)
		peer, found := n.peers.getByIdx(int(sampledIdx))
		if !found {
			return res, fmt.Errorf("no peer at index %v", sampledIdx)
		}

		peerIP := peer.getIP()
		switch {
		case !peer.finishedHandshake.GetValue():
			continue
		case peerIP.IsZero():
			continue
		case !n.vdrs.Contains(peer.nodeID):
			continue
		}

		peerVersion := peer.versionStruct.GetValue().(version.Application)
		if n.versionCompatibility.Unmaskable(peerVersion) != nil {
			continue
		}

		signedIP, ok := peer.sigAndTime.GetValue().(signedPeerIP)
		if !ok || !signedIP.ip.Equal(peerIP) {
			continue
		}

		res = append(res, utils.IPCertDesc{
			IPDesc:    peerIP,
			Signature: signedIP.signature,
			Cert:      peer.cert,
			Time:      signedIP.time,
		})
	}

	return res, nil
}

// Should only be called after the peer finishes the handshake.
// Should not be called after [disconnected] is called with this peer.
// Assumes [n.stateLock] is not held.
func (n *network) connected(p *peer) {
	p.net.stateLock.Lock()
	defer p.net.stateLock.Unlock()

	p.finishedHandshake.SetValue(true)

	peerVersion := p.versionStruct.GetValue().(version.Application)

	if n.hasMasked {
		if n.versionCompatibility.Unmaskable(peerVersion) != nil {
			if err := n.vdrs.MaskValidator(p.nodeID); err != nil {
				n.log.Error("failed to mask validator %s due to %s", p.nodeID, err)
			}
		} else {
			if err := n.vdrs.RevealValidator(p.nodeID); err != nil {
				n.log.Error("failed to reveal validator %s due to %s", p.nodeID, err)
			}
		}
		n.log.Verbo("The new staking set is:\n%s", n.vdrs)
	} else {
		if n.versionCompatibility.WontMask(peerVersion) != nil {
			n.maskedValidators.Add(p.nodeID)
		} else {
			n.maskedValidators.Remove(p.nodeID)
		}
	}

	// TODO also delete an entry from this map when they leave the validator set
	delete(n.latestPeerIP, p.nodeID)

	ip := p.getIP()
	n.log.Debug("connected to %s at %s", p.nodeID, ip)

	if !ip.IsZero() {
		str := ip.String()

		delete(n.disconnectedIPs, str)
		delete(n.retryDelay, str)
		n.connectedIPs[str] = struct{}{}
	}

	n.router.Connected(p.nodeID)
}

// should only be called after the peer is marked as connected.
// Assumes [n.stateLock] is not held.
func (n *network) disconnected(p *peer) {
	p.net.stateLock.Lock()
	defer p.net.stateLock.Unlock()

	ip := p.getIP()

	n.log.Debug("disconnected from %s at %s", p.nodeID, ip)

	n.peers.remove(p)
	n.numPeers.Set(float64(n.peers.size()))

	p.releaseAllAliases()

	if !ip.IsZero() {
		str := ip.String()

		delete(n.disconnectedIPs, str)
		delete(n.connectedIPs, str)

		if n.vdrs.Contains(p.nodeID) {
			n.track(ip, p.nodeID)
		}
	}

	// Only send Disconnected to router if Connected was sent
	if p.finishedHandshake.GetValue() {
		n.router.Disconnected(p.nodeID)
	}
}

// holds onto the peer object as a result of helper functions
type PeerElement struct {
	// the peer, if it wasn't a peer when we cloned the list this value will be
	// nil
	peer *peer
	// this is the validator id for the peer, we pass back to the caller for
	// logging purposes
	id ids.ShortID
}

// Safe copy the peers dressed as a PeerElement
// Assumes [n.stateLock] is not held.
func (n *network) getPeers(nodeIDs ids.ShortSet) []*PeerElement {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if n.closed.GetValue() {
		return nil
	}

	peers := make([]*PeerElement, nodeIDs.Len())
	i := 0
	for nodeID := range nodeIDs {
		nodeID := nodeID                   // Prevent overwrite in next loop iteration
		peer, _ := n.peers.getByID(nodeID) // note: peer may be nil
		peers[i] = &PeerElement{
			peer: peer,
			id:   nodeID,
		}
		i++
	}

	return peers
}

// Returns a copy of the peer set.
// Only includes peers that have finished the handshake.
// Assumes [n.stateLock] is not held.
func (n *network) getAllPeers() []*peer {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if n.closed.GetValue() {
		return nil
	}

	peers := make([]*peer, 0, n.peers.size())
	for _, peer := range n.peers.peersList {
		if peer.finishedHandshake.GetValue() {
			peers = append(peers, peer)
		}
	}
	return peers
}

// Safe find a single peer
// Assumes [n.stateLock] is not held.
func (n *network) getPeer(nodeID ids.ShortID) *peer {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if n.closed.GetValue() {
		return nil
	}

	res, _ := n.peers.getByID(nodeID) // note: peer may be nil
	return res
}

// HealthCheck returns information about several network layer health checks.
// 1) Information about health check results
// 2) An error if the health check reports unhealthy
// Assumes [n.stateLock] is not held
func (n *network) HealthCheck() (interface{}, error) {
	// Get some data with the state lock held
	connectedTo := 0
	n.stateLock.RLock()
	for _, peer := range n.peers.peersList {
		if peer != nil && peer.finishedHandshake.GetValue() {
			connectedTo++
		}
	}
	pendingSendBytes := atomic.LoadInt64(&n.pendingBytes)
	sendFailRate := n.sendFailRateCalculator.Read()
	n.stateLock.RUnlock()

	// Make sure we're connected to at least the minimum number of peers
	healthy := connectedTo >= int(n.healthConfig.MinConnectedPeers)
	details := map[string]interface{}{
		"connectedPeers": connectedTo,
	}

	// Make sure we've received an incoming message within the threshold
	now := n.clock.Time()

	lastMsgReceivedAt := time.Unix(atomic.LoadInt64(&n.lastMsgReceivedTime), 0)
	timeSinceLastMsgReceived := now.Sub(lastMsgReceivedAt)
	healthy = healthy && timeSinceLastMsgReceived <= n.healthConfig.MaxTimeSinceMsgReceived
	details["timeSinceLastMsgReceived"] = timeSinceLastMsgReceived.String()
	n.metrics.timeSinceLastMsgReceived.Set(float64(timeSinceLastMsgReceived.Milliseconds()))

	// Make sure we've sent an outgoing message within the threshold
	lastMsgSentAt := time.Unix(atomic.LoadInt64(&n.lastMsgSentTime), 0)
	timeSinceLastMsgSent := now.Sub(lastMsgSentAt)
	healthy = healthy && timeSinceLastMsgSent <= n.healthConfig.MaxTimeSinceMsgSent
	details["timeSinceLastMsgSent"] = timeSinceLastMsgSent.String()
	n.metrics.timeSinceLastMsgSent.Set(float64(timeSinceLastMsgSent.Milliseconds()))

	// Make sure the send queue isn't too full
	portionFull := float64(pendingSendBytes) / float64(n.maxNetworkPendingSendBytes) // In [0,1]
	healthy = healthy && portionFull <= n.healthConfig.MaxPortionSendQueueBytesFull
	details["sendQueuePortionFull"] = portionFull
	n.metrics.sendQueuePortionFull.Set(portionFull)

	// Make sure the message send failed rate isn't too high
	healthy = healthy && sendFailRate <= n.healthConfig.MaxSendFailRate
	details["sendFailRate"] = sendFailRate
	n.metrics.sendFailRate.Set(sendFailRate)

	// Network layer is unhealthy
	if !healthy {
		return details, errNetworkLayerUnhealthy
	}
	return details, nil
}

// assume [n.stateLock] is held. Returns the timestamp and signature that should
// be sent in a Version message. We only update these values when our IP has
// changed.
func (n *network) getVersion(ip utils.IPDesc) (uint64, []byte, error) {
	n.timeForIPLock.Lock()
	defer n.timeForIPLock.Unlock()

	if !ip.Equal(n.lastVersionIP) {
		newTimestamp := n.clock.Unix()
		msgHash := ipAndTimeHash(ip, newTimestamp)
		sig, err := n.tlsKey.Sign(cryptorand.Reader, msgHash, crypto.SHA256)
		if err != nil {
			return 0, nil, err
		}

		n.lastVersionIP = ip
		n.lastVersionTimestamp = newTimestamp
		n.lastVersionSignature = sig
	}

	return n.lastVersionTimestamp, n.lastVersionSignature, nil
}
