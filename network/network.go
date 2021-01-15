// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/api/health"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/networking/router"
	"github.com/ava-labs/avalanchego/snow/networking/sender"
	"github.com/ava-labs/avalanchego/snow/triggers"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/sampler"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/version"
)

// reasonable default values
const (
	defaultInitialReconnectDelay                     = time.Second
	defaultMaxReconnectDelay                         = time.Hour
	DefaultMaxMessageSize                     uint32 = 1 << 21
	defaultMaxNetworkPendingSendBytes                = 1 << 29 // 512MB
	defaultNetworkPendingSendBytesToRateLimit        = defaultMaxNetworkPendingSendBytes / 4
	defaultMaxClockDifference                        = time.Minute
	defaultPeerListGossipSpacing                     = time.Minute
	defaultPeerListGossipSize                        = 100
	defaultPeerListStakerGossipFraction              = 2
	defaultGetVersionTimeout                         = 2 * time.Second
	defaultAllowPrivateIPs                           = true
	defaultGossipSize                                = 50
	defaultPingPongTimeout                           = time.Minute
	defaultPingFrequency                             = 3 * defaultPingPongTimeout / 4
	defaultReadBufferSize                            = 16 * 1024
	defaultReadHandshakeTimeout                      = 15 * time.Second
	defaultConnMeterCacheSize                        = 10000
)

var (
	errNetworkClosed = errors.New("network closed")
	errPeerIsMyself  = errors.New("peer is myself")

	minimumUnmaskedVersion = version.NewDefaultVersion(constants.PlatformName, 1, 2, 0)
)

func init() { rand.Seed(time.Now().UnixNano()) }

// Network defines the functionality of the networking library.
type Network interface {
	// All consensus messages can be sent through this interface. Thread safety
	// must be managed internally in the network.
	sender.ExternalSender

	// The network must be able to broadcast accepted decisions to random peers.
	// Thread safety must be managed internally in the network.
	triggers.Acceptor

	// The network should be able to report the last time the network interacted
	// with a peer
	health.Heartbeater

	// Should only be called once, will run until either a fatal error occurs,
	// or the network is closed. Returns a non-nil error.
	Dispatch() error

	// Attempt to connect to this IP. Thread safety must be managed internally
	// to the network. The network will never stop attempting to connect to this
	// IP.
	Track(ip utils.IPDesc)

	// Returns the description of the nodes this network is currently connected
	// to externally. Thread safety must be managed internally to the network.
	Peers() []PeerID

	// Close this network and all existing connections it has. Thread safety
	// must be managed internally to the network. Calling close multiple times
	// will return a nil error.
	Close() error

	// Return the IP of the node
	IP() utils.IPDesc
}

type network struct {
	// The metrics that this network tracks
	metrics

	log                                logging.Logger
	id                                 ids.ShortID
	ip                                 utils.DynamicIPDesc
	networkID                          uint32
	version                            version.Version
	parser                             version.Parser
	listener                           net.Listener
	dialer                             Dialer
	serverUpgrader                     Upgrader
	clientUpgrader                     Upgrader
	vdrs                               validators.Set // set of current validators in the Avalanche network
	beacons                            validators.Set // set of beacons in the Avalanche network
	router                             router.Router  // router must be thread safe
	nodeID                             uint32
	clock                              timer.Clock
	lastHeartbeat                      int64
	initialReconnectDelay              time.Duration
	maxReconnectDelay                  time.Duration
	maxMessageSize                     int64
	sendQueueSize                      uint32
	maxNetworkPendingSendBytes         int64
	networkPendingSendBytesToRateLimit int64
	maxClockDifference                 time.Duration
	peerListGossipSpacing              time.Duration
	peerListGossipSize                 int
	peerListStakerGossipFraction       int
	getVersionTimeout                  time.Duration
	allowPrivateIPs                    bool
	gossipSize                         int
	pingPongTimeout                    time.Duration
	pingFrequency                      time.Duration
	readBufferSize                     uint32
	readHandshakeTimeout               time.Duration
	connMeterMaxConns                  int
	connMeter                          ConnMeter
	executor                           timer.Executor
	b                                  Builder
	maskTime                           time.Time

	// stateLock should never be held when grabbing a peer lock
	stateLock       sync.RWMutex
	pendingBytes    int64
	closed          utils.AtomicBool
	disconnectedIPs map[string]struct{}
	connectedIPs    map[string]struct{}
	retryDelay      map[string]time.Duration
	// TODO: bound the size of [myIPs] to avoid DoS. LRU caching would be ideal
	myIPs map[string]struct{} // set of IPs that resulted in my ID.
	peers map[ids.ShortID]*peer

	// ensures the close of the network only happens once.
	closeOnce sync.Once

	// True if the node should restart if it detects it's disconnected from all peers
	restartOnDisconnected bool

	// Signals the connection checker to close when Network is shutdown.
	// See restartOnDisconnect()
	connectedCheckerCloser chan struct{}

	// Used to monitor whether the node is connected to peers. If the node has
	// been connected to at least one peer in the last [disconnectedRestartTimeout]
	// then connectedMeter.Ticks() is non-zero.
	connectedMeter timer.TimedMeter

	// How often we check that we're connected to at least one peer.
	// Used to update [connectedMeter].
	// If 0, node will not restart even if it has no peers.
	disconnectedCheckFreq time.Duration

	// restarter can shutdown and restart the node.
	// If nil, node will not restart even if it has no peers.
	restarter utils.Restarter

	hasMasked        bool
	maskedValidators ids.ShortSet
}

// NewDefaultNetwork returns a new Network implementation with the provided
// parameters and some reasonable default values.
func NewDefaultNetwork(
	registerer prometheus.Registerer,
	log logging.Logger,
	id ids.ShortID,
	ip utils.DynamicIPDesc,
	networkID uint32,
	version version.Version,
	parser version.Parser,
	listener net.Listener,
	dialer Dialer,
	serverUpgrader,
	clientUpgrader Upgrader,
	vdrs validators.Set,
	beacons validators.Set,
	router router.Router,
	connMeterResetDuration time.Duration,
	connMeterMaxConns int,
	restarter utils.Restarter,
	restartOnDisconnected bool,
	disconnectedCheckFreq time.Duration,
	disconnectedRestartTimeout time.Duration,
	apricotPhase0Time time.Time,
	sendQueueSize uint32,
) Network {
	return NewNetwork(
		registerer,
		log,
		id,
		ip,
		networkID,
		version,
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
		defaultPeerListGossipSpacing,
		defaultPeerListGossipSize,
		defaultPeerListStakerGossipFraction,
		defaultGetVersionTimeout,
		defaultAllowPrivateIPs,
		defaultGossipSize,
		defaultPingPongTimeout,
		defaultPingFrequency,
		defaultReadBufferSize,
		defaultReadHandshakeTimeout,
		connMeterResetDuration,
		defaultConnMeterCacheSize,
		connMeterMaxConns,
		restarter,
		restartOnDisconnected,
		disconnectedCheckFreq,
		disconnectedRestartTimeout,
		apricotPhase0Time,
	)
}

// NewNetwork returns a new Network implementation with the provided parameters.
func NewNetwork(
	registerer prometheus.Registerer,
	log logging.Logger,
	id ids.ShortID,
	ip utils.DynamicIPDesc,
	networkID uint32,
	version version.Version,
	parser version.Parser,
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
	peerListGossipSpacing time.Duration,
	peerListGossipSize int,
	peerListStakerGossipFraction int,
	getVersionTimeout time.Duration,
	allowPrivateIPs bool,
	gossipSize int,
	pingPongTimeout time.Duration,
	pingFrequency time.Duration,
	readBufferSize uint32,
	readHandshakeTimeout time.Duration,
	connMeterResetDuration time.Duration,
	connMeterCacheSize int,
	connMeterMaxConns int,
	restarter utils.Restarter,
	restartOnDisconnected bool,
	disconnectedCheckFreq time.Duration,
	disconnectedRestartTimeout time.Duration,
	maskTime time.Time,
) Network {
	// #nosec G404
	netw := &network{
		log:            log,
		id:             id,
		ip:             ip,
		networkID:      networkID,
		version:        version,
		parser:         parser,
		listener:       listener,
		dialer:         dialer,
		serverUpgrader: serverUpgrader,
		clientUpgrader: clientUpgrader,
		vdrs:           vdrs,
		beacons:        beacons,
		router:         router,
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
		peerListGossipSpacing:              peerListGossipSpacing,
		peerListGossipSize:                 peerListGossipSize,
		peerListStakerGossipFraction:       peerListStakerGossipFraction,
		getVersionTimeout:                  getVersionTimeout,
		allowPrivateIPs:                    allowPrivateIPs,
		gossipSize:                         gossipSize,
		pingPongTimeout:                    pingPongTimeout,
		pingFrequency:                      pingFrequency,
		disconnectedIPs:                    make(map[string]struct{}),
		connectedIPs:                       make(map[string]struct{}),
		retryDelay:                         make(map[string]time.Duration),
		myIPs:                              map[string]struct{}{ip.IP().String(): {}},
		peers:                              make(map[ids.ShortID]*peer),
		readBufferSize:                     readBufferSize,
		readHandshakeTimeout:               readHandshakeTimeout,
		connMeter:                          NewConnMeter(connMeterResetDuration, connMeterCacheSize),
		connMeterMaxConns:                  connMeterMaxConns,
		restartOnDisconnected:              restartOnDisconnected,
		connectedCheckerCloser:             make(chan struct{}),
		disconnectedCheckFreq:              disconnectedCheckFreq,
		connectedMeter:                     timer.TimedMeter{Duration: disconnectedRestartTimeout},
		restarter:                          restarter,
		maskTime:                           maskTime,
	}

	if err := netw.initialize(registerer); err != nil {
		log.Warn("initializing network metrics failed with: %s", err)
	}
	netw.executor.Initialize()
	go netw.executor.Dispatch()
	netw.heartbeat()
	if restartOnDisconnected && disconnectedCheckFreq != 0 && disconnectedRestartTimeout != 0 {
		log.Info("node will restart if not connected to any peers")
		// pre-queue one tick to avoid immediate shutdown.
		netw.connectedMeter.Tick()
		go netw.restartOnDisconnect()
	}
	return netw
}

// GetAcceptedFrontier implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) GetAcceptedFrontier(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Time) {
	msg, err := n.b.GetAcceptedFrontier(chainID, requestID, uint64(deadline.Sub(n.clock.Time())))
	n.log.AssertNoError(err)

	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
			n.log.Debug("failed to send GetAcceptedFrontier(%s, %s, %d)",
				vID,
				chainID,
				requestID)
			n.executor.Add(func() { n.router.GetAcceptedFrontierFailed(vID, chainID, requestID) })
			n.getAcceptedFrontier.numFailed.Inc()
		} else {
			n.getAcceptedFrontier.numSent.Inc()
		}
	}
}

// AcceptedFrontier implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) AcceptedFrontier(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID) {
	msg, err := n.b.AcceptedFrontier(chainID, requestID, containerIDs)
	if err != nil {
		n.log.Error("failed to build AcceptedFrontier(%s, %d, %s): %s",
			chainID,
			requestID,
			containerIDs,
			err)
		return // Packing message failed
	}

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send AcceptedFrontier(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			containerIDs)
		n.acceptedFrontier.numFailed.Inc()
	} else {
		n.acceptedFrontier.numSent.Inc()
	}
}

// GetAccepted implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) GetAccepted(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Time, containerIDs []ids.ID) {
	msg, err := n.b.GetAccepted(chainID, requestID, uint64(deadline.Sub(n.clock.Time())), containerIDs)
	if err != nil {
		n.log.Error("failed to build GetAccepted(%s, %d, %s): %s",
			chainID,
			requestID,
			containerIDs,
			err)
		for validatorID := range validatorIDs {
			vID := validatorID // Prevent overwrite in next loop iteration
			n.executor.Add(func() {
				n.router.GetAcceptedFailed(vID, chainID, requestID)
			})
		}
		return
	}

	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
			n.log.Debug("failed to send GetAccepted(%s, %s, %d, %s)",
				vID,
				chainID,
				requestID,
				containerIDs)
			n.executor.Add(func() { n.router.GetAcceptedFailed(vID, chainID, requestID) })
			n.getAccepted.numFailed.Inc()
		} else {
			n.getAccepted.numSent.Inc()
		}
	}
}

// Accepted implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) Accepted(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerIDs []ids.ID) {
	msg, err := n.b.Accepted(chainID, requestID, containerIDs)
	if err != nil {
		n.log.Error("failed to build Accepted(%s, %d, %s): %s",
			chainID,
			requestID,
			containerIDs,
			err)
		return // Packing message failed
	}

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send Accepted(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			containerIDs)
		n.accepted.numFailed.Inc()
	} else {
		n.accepted.numSent.Inc()
	}
}

// GetAncestors implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) GetAncestors(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID) {
	msg, err := n.b.GetAncestors(chainID, requestID, uint64(deadline.Sub(n.clock.Time())), containerID)
	if err != nil {
		n.log.Error("failed to build GetAncestors message: %s", err)
		return
	}

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send GetAncestors(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			containerID)
		n.executor.Add(func() { n.router.GetAncestorsFailed(validatorID, chainID, requestID) })
		n.getAncestors.numFailed.Inc()
	} else {
		n.getAncestors.numSent.Inc()
	}
}

// MultiPut implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) MultiPut(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containers [][]byte) {
	msg, err := n.b.MultiPut(chainID, requestID, containers)
	if err != nil {
		n.log.Error("failed to build MultiPut message because of container of size %d", len(containers))
		return
	}

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send MultiPut(%s, %s, %d, %d)",
			validatorID,
			chainID,
			requestID,
			len(containers))
		n.multiPut.numFailed.Inc()
	} else {
		n.multiPut.numSent.Inc()
	}
}

// Get implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) Get(validatorID ids.ShortID, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID) {
	msg, err := n.b.Get(chainID, requestID, uint64(deadline.Sub(n.clock.Time())), containerID)
	n.log.AssertNoError(err)

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send Get(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			containerID)
		n.executor.Add(func() { n.router.GetFailed(validatorID, chainID, requestID) })
		n.get.numFailed.Inc()
	} else {
		n.get.numSent.Inc()
	}
}

// Put implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) Put(validatorID ids.ShortID, chainID ids.ID, requestID uint32, containerID ids.ID, container []byte) {
	msg, err := n.b.Put(chainID, requestID, containerID, container)
	if err != nil {
		n.log.Error("failed to build Put(%s, %d, %s): %s. len(container) : %d",
			chainID,
			requestID,
			containerID,
			err,
			len(container))
		return
	}

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send Put(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			containerID)
		n.log.Verbo("container: %s", formatting.DumpBytes{Bytes: container})
		n.put.numFailed.Inc()
	} else {
		n.put.numSent.Inc()
	}
}

// PushQuery implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) PushQuery(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID, container []byte) {
	msg, err := n.b.PushQuery(chainID, requestID, uint64(deadline.Sub(n.clock.Time())), containerID, container)

	if err != nil {
		n.log.Error("failed to build PushQuery(%s, %d, %s): %s. len(container): %d",
			chainID,
			requestID,
			containerID,
			err,
			len(container))
		n.log.Verbo("container: %s", formatting.DumpBytes{Bytes: container})
		for validatorID := range validatorIDs {
			vID := validatorID // Prevent overwrite in next loop iteration
			n.executor.Add(func() { n.router.QueryFailed(vID, chainID, requestID) })
		}
		return // Packing message failed
	}

	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
			n.log.Debug("failed to send PushQuery(%s, %s, %d, %s)",
				vID,
				chainID,
				requestID,
				containerID)
			n.log.Verbo("container: %s", formatting.DumpBytes{Bytes: container})
			n.executor.Add(func() { n.router.QueryFailed(vID, chainID, requestID) })
			n.pushQuery.numFailed.Inc()
		} else {
			n.pushQuery.numSent.Inc()
		}
	}
}

// PullQuery implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) PullQuery(validatorIDs ids.ShortSet, chainID ids.ID, requestID uint32, deadline time.Time, containerID ids.ID) {
	msg, err := n.b.PullQuery(chainID, requestID, uint64(deadline.Sub(n.clock.Time())), containerID)
	n.log.AssertNoError(err)

	for _, peerElement := range n.getPeers(validatorIDs) {
		peer := peerElement.peer
		vID := peerElement.id
		if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
			n.log.Debug("failed to send PullQuery(%s, %s, %d, %s)",
				vID,
				chainID,
				requestID,
				containerID)
			n.executor.Add(func() { n.router.QueryFailed(vID, chainID, requestID) })
			n.pullQuery.numFailed.Inc()
		} else {
			n.pullQuery.numSent.Inc()
		}
	}
}

// Chits implements the Sender interface.
// assumes the stateLock is not held.
func (n *network) Chits(validatorID ids.ShortID, chainID ids.ID, requestID uint32, votes []ids.ID) {
	msg, err := n.b.Chits(chainID, requestID, votes)
	if err != nil {
		n.log.Error("failed to build Chits(%s, %d, %s): %s",
			chainID,
			requestID,
			votes,
			err)
		return
	}

	peer := n.getPeer(validatorID)
	if peer == nil || !peer.connected.GetValue() || !peer.Send(msg) {
		n.log.Debug("failed to send Chits(%s, %s, %d, %s)",
			validatorID,
			chainID,
			requestID,
			votes)
		n.chits.numFailed.Inc()
	} else {
		n.chits.numSent.Inc()
	}
}

// Gossip attempts to gossip the container to the network
// assumes the stateLock is not held.
func (n *network) Gossip(chainID, containerID ids.ID, container []byte) {
	if err := n.gossipContainer(chainID, containerID, container); err != nil {
		n.log.Debug("failed to Gossip(%s, %s): %s", chainID, containerID, err)
		n.log.Verbo("container:\n%s", formatting.DumpBytes{Bytes: container})
	}
}

// Accept is called after every consensus decision
// assumes the stateLock is not held.
func (n *network) Accept(ctx *snow.Context, containerID ids.ID, container []byte) error {
	if !ctx.IsBootstrapped() {
		// don't gossip during bootstrapping
		return nil
	}
	return n.gossipContainer(ctx.ChainID, containerID, container)
}

// heartbeat registers a new heartbeat to signal liveness
func (n *network) heartbeat() { atomic.StoreInt64(&n.lastHeartbeat, n.clock.Time().Unix()) }

// GetHeartbeat returns the most recent heartbeat time
func (n *network) GetHeartbeat() int64 { return atomic.LoadInt64(&n.lastHeartbeat) }

// Dispatch starts accepting connections from other nodes attempting to connect
// to this node.
// assumes the stateLock is not held.
func (n *network) Dispatch() error {
	go n.gossip() // Periodically gossip peers
	go func() {
		duration := time.Until(n.maskTime)
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
		if conn, ok := conn.(*net.TCPConn); ok {
			if err := conn.SetLinger(0); err != nil {
				n.log.Warn("failed to set no linger due to: %s", err)
			}
			if err := conn.SetNoDelay(true); err != nil {
				n.log.Warn("failed to set socket nodelay due to: %s", err)
			}
		}

		addr := conn.RemoteAddr().String()
		ticks, err := n.connMeter.Register(addr)
		// looking for > n.connMeterMaxConns indicating the second tick
		if err == nil && ticks > n.connMeterMaxConns {
			n.log.Debug("connection from %s temporarily dropped", addr)
			_ = conn.Close()
			continue
		}

		go func() {
			err := n.upgrade(
				&peer{
					net:          n,
					conn:         conn,
					tickerCloser: make(chan struct{}),
				},
				n.serverUpgrader,
			)
			if err != nil {
				n.log.Verbo("failed to upgrade connection: %s", err)
			}
		}()
	}
}

// IPs implements the Network interface
// assumes the stateLock is not held.
func (n *network) Peers() []PeerID {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	peers := make([]PeerID, 0, len(n.peers))
	for _, peer := range n.peers {
		if peer.connected.GetValue() {
			peers = append(peers, PeerID{
				IP:           peer.conn.RemoteAddr().String(),
				PublicIP:     peer.getIP().String(),
				ID:           peer.id.PrefixedString(constants.NodeIDPrefix),
				Version:      peer.versionStr.GetValue().(string),
				LastSent:     time.Unix(atomic.LoadInt64(&peer.lastSent), 0),
				LastReceived: time.Unix(atomic.LoadInt64(&peer.lastReceived), 0),
			})
		}
	}
	return peers
}

// Close implements the Network interface
// assumes the stateLock is not held.
func (n *network) Close() error {
	n.closeOnce.Do(n.close)
	return nil
}

func (n *network) close() {
	n.log.Info("shutting down network")
	// Stop checking whether we're connected to peers.
	close(n.connectedCheckerCloser)

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

	peersToClose := make([]*peer, len(n.peers))
	i := 0
	for _, peer := range n.peers {
		peersToClose[i] = peer
		i++
	}
	n.peers = make(map[ids.ShortID]*peer)
	n.stateLock.Unlock()

	for _, peer := range peersToClose {
		peer.Close() // Grabs the stateLock
	}
}

// Track implements the Network interface
// assumes the stateLock is not held.
func (n *network) Track(ip utils.IPDesc) {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	n.track(ip)
}

func (n *network) IP() utils.IPDesc {
	return n.ip.IP()
}

// assumes the stateLock is not held.
func (n *network) gossipContainer(chainID, containerID ids.ID, container []byte) error {
	msg, err := n.b.Put(chainID, constants.GossipMsgRequestID, containerID, container)
	if err != nil {
		return fmt.Errorf("attempted to pack too large of a Put message.\nContainer length: %d", len(container))
	}

	allPeers := n.getAllPeers()

	numToGossip := n.gossipSize
	if numToGossip > len(allPeers) {
		numToGossip = len(allPeers)
	}

	s := sampler.NewUniform()
	if err := s.Initialize(uint64(len(allPeers))); err != nil {
		return err
	}
	indices, err := s.Sample(numToGossip)
	if err != nil {
		return err
	}
	for _, index := range indices {
		if allPeers[int(index)].Send(msg) {
			n.put.numSent.Inc()
		} else {
			n.put.numFailed.Inc()
		}
	}
	return nil
}

// assumes the stateLock is held.
func (n *network) track(ip utils.IPDesc) {
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
	if _, ok := n.myIPs[str]; ok {
		return
	}
	n.disconnectedIPs[str] = struct{}{}

	go n.connectTo(ip)
}

// assumes the stateLock is not held. Only returns after the network is closed.
func (n *network) gossip() {
	t := time.NewTicker(n.peerListGossipSpacing)
	defer t.Stop()

	for range t.C {
		if n.closed.GetValue() {
			return
		}

		allPeers := n.getAllPeers()
		if len(allPeers) == 0 {
			continue
		}

		ips := make([]utils.IPDesc, 0, len(allPeers))
		for _, peer := range allPeers {
			ip := peer.getIP()
			if peer.connected.GetValue() &&
				!ip.IsZero() &&
				n.vdrs.Contains(peer.id) {
				peerVersion := peer.versionStruct.GetValue().(version.Version)
				if !peerVersion.Before(minimumUnmaskedVersion) || time.Since(n.maskTime) < 0 {
					ips = append(ips, ip)
				}
			}
		}

		if len(ips) == 0 {
			n.log.Debug("skipping validator gossiping as no public validators are connected")
			continue
		}
		msg, err := n.b.PeerList(ips)
		if err != nil {
			n.log.Error("failed to build peer list to gossip: %s. len(ips): %d",
				err,
				len(ips))
			continue
		}

		stakers := make([]*peer, 0, len(allPeers))
		nonStakers := make([]*peer, 0, len(allPeers))
		for _, peer := range allPeers {
			if n.vdrs.Contains(peer.id) {
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
		for _, index := range stakerIndices {
			stakers[int(index)].Send(msg)
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
		for _, index := range nonStakerIndices {
			nonStakers[int(index)].Send(msg)
		}
	}
}

// assumes the stateLock is not held. Only returns if the ip is connected to or
// the network is closed
func (n *network) connectTo(ip utils.IPDesc) {
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
		closed := n.closed

		if !isDisconnected || isConnected || isMyself || closed.GetValue() {
			// If the IP was discovered by the peer connecting to us, we don't
			// need to attempt to connect anymore

			// If the IP was discovered to be our IP address, we don't need to
			// attempt to connect anymore

			// If the network was closed, we should stop attempting to connect
			// to the peer

			n.stateLock.Unlock()
			return
		}
		n.retryDelay[str] = delay
		n.stateLock.Unlock()

		err := n.attemptConnect(ip)
		if err == nil {
			return
		}
		n.log.Verbo("error attempting to connect to %s: %s. Reattempting in %s",
			ip, err, delay)
	}
}

// assumes the stateLock is not held. Returns nil if a connection was able to be
// established, or the network is closed.
func (n *network) attemptConnect(ip utils.IPDesc) error {
	n.log.Verbo("attempting to connect to %s", ip)

	conn, err := n.dialer.Dial(ip)
	if err != nil {
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
	return n.upgrade(&peer{
		net:          n,
		ip:           ip,
		conn:         conn,
		tickerCloser: make(chan struct{}),
	}, n.clientUpgrader)
}

// assumes the stateLock is not held. Returns an error if the peer's connection
// wasn't able to be upgraded.
func (n *network) upgrade(p *peer, upgrader Upgrader) error {
	if err := p.conn.SetReadDeadline(time.Now().Add(n.readHandshakeTimeout)); err != nil {
		_ = p.conn.Close()
		n.log.Verbo("failed to set the read deadline with %s", err)
		return err
	}

	id, conn, err := upgrader.Upgrade(p.conn)
	if err != nil {
		_ = p.conn.Close()
		n.log.Verbo("failed to upgrade connection with %s", err)
		return err
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		_ = p.conn.Close()
		n.log.Verbo("failed to clear the read deadline with %s", err)
		return err
	}

	p.sender = make(chan []byte, n.sendQueueSize)
	p.id = id
	p.conn = conn

	if err := n.tryAddPeer(p); err != nil {
		_ = p.conn.Close()
		n.log.Debug("dropping peer connection due to: %s", err)
	}
	return nil
}

// assumes the stateLock is not held. Returns an error if the peer couldn't be
// added.
func (n *network) tryAddPeer(p *peer) error {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	ip := p.getIP()

	if n.closed.GetValue() {
		// the network is closing, so make sure that no further reconnect
		// attempts are made.
		return errNetworkClosed
	}

	// if this connection is myself, then I should delete the connection and
	// mark the IP as one of mine.
	if p.id == n.id {
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
	// connection.
	if _, ok := n.peers[p.id]; ok {
		if !ip.IsZero() {
			str := ip.String()
			delete(n.disconnectedIPs, str)
			delete(n.retryDelay, str)
		}
		return fmt.Errorf("duplicated connection from %s at %s", p.id.PrefixedString(constants.NodeIDPrefix), ip)
	}

	n.peers[p.id] = p
	n.numPeers.Set(float64(len(n.peers)))
	p.Start()
	return nil
}

// assumes the stateLock is not held. Returns the ips of connections that have
// valid IPs that are marked as validators.
func (n *network) validatorIPs() []utils.IPDesc {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	ips := make([]utils.IPDesc, 0, len(n.peers))
	for _, peer := range n.peers {
		ip := peer.getIP()
		if peer.connected.GetValue() && !ip.IsZero() && n.vdrs.Contains(peer.id) {
			peerVersion := peer.versionStruct.GetValue().(version.Version)
			if !peerVersion.Before(minimumUnmaskedVersion) || time.Since(n.maskTime) < 0 {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

// should only be called after the peer is marked as connected. Should not be
// called after disconnected is called with this peer.
// assumes the stateLock is not held.
func (n *network) connected(p *peer) {
	p.net.stateLock.Lock()
	defer p.net.stateLock.Unlock()

	p.connected.SetValue(true)

	peerVersion := p.versionStruct.GetValue().(version.Version)

	if n.hasMasked {
		if peerVersion.Before(minimumUnmaskedVersion) {
			if err := n.vdrs.MaskValidator(p.id); err != nil {
				n.log.Error("failed to mask validator %s due to %s", p.id, err)
			}
		} else {
			if err := n.vdrs.RevealValidator(p.id); err != nil {
				n.log.Error("failed to reveal validator %s due to %s", p.id, err)
			}
		}
		n.log.Verbo("The new staking set is:\n%s", n.vdrs)
	} else {
		if peerVersion.Before(minimumUnmaskedVersion) {
			n.maskedValidators.Add(p.id)
		} else {
			n.maskedValidators.Remove(p.id)
		}
	}

	ip := p.getIP()
	n.log.Debug("connected to %s at %s", p.id, ip)

	if !ip.IsZero() {
		str := ip.String()

		delete(n.disconnectedIPs, str)
		delete(n.retryDelay, str)
		n.connectedIPs[str] = struct{}{}
	}

	n.router.Connected(p.id)
}

// should only be called after the peer is marked as connected.
// assumes the stateLock is not held.
func (n *network) disconnected(p *peer) {
	p.net.stateLock.Lock()
	defer p.net.stateLock.Unlock()

	ip := p.getIP()

	n.log.Debug("disconnected from %s at %s", p.id, ip)

	delete(n.peers, p.id)
	n.numPeers.Set(float64(len(n.peers)))

	if !ip.IsZero() {
		str := ip.String()

		delete(n.disconnectedIPs, str)
		delete(n.connectedIPs, str)

		n.track(ip)
	}

	if p.connected.GetValue() {
		n.router.Disconnected(p.id)
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
// assumes the stateLock is not held.
func (n *network) getPeers(validatorIDs ids.ShortSet) []*PeerElement {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if n.closed.GetValue() {
		return nil
	}

	peers := make([]*PeerElement, validatorIDs.Len())
	i := 0
	for validatorID := range validatorIDs {
		vID := validatorID // Prevent overwrite in next loop iteration
		peers[i] = &PeerElement{
			peer: n.peers[vID],
			id:   vID,
		}
		i++
	}

	return peers
}

// Safe copy the peers. Assumes the stateLock is not held.
func (n *network) getAllPeers() []*peer {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if n.closed.GetValue() {
		return nil
	}

	peers := make([]*peer, len(n.peers))
	i := 0
	for _, peer := range n.peers {
		peers[i] = peer
		i++
	}
	return peers
}

// Safe find a single peer
// assumes the stateLock is not held.
func (n *network) getPeer(validatorID ids.ShortID) *peer {
	n.stateLock.RLock()
	defer n.stateLock.RUnlock()

	if n.closed.GetValue() {
		return nil
	}
	return n.peers[validatorID]
}

// restartOnDisconnect checks every [n.disconnectedCheckFreq] whether this node is connected
// to any peers. If the node is not connected to any peers for [disconnectedRestartTimeout],
// restarts the node.
func (n *network) restartOnDisconnect() {
	ticker := time.NewTicker(n.disconnectedCheckFreq)
	for {
		select {
		case <-ticker.C:
			if n.closed.GetValue() {
				return
			}
			n.stateLock.RLock()
			for _, peer := range n.peers {
				if peer != nil && peer.connected.GetValue() {
					n.connectedMeter.Tick()
					break
				}
			}
			n.stateLock.RUnlock()
			if n.connectedMeter.Ticks() != 0 {
				continue
			}
			ticker.Stop()
			n.log.Info("restarting node due to no peers")
			go n.restarter.Restart()
		case <-n.connectedCheckerCloser:
			ticker.Stop()
			return
		}
	}
}
