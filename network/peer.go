// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"encoding/binary"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/version"
)

// alias is a secondary IP address where a peer
// was reached
type alias struct {
	// ip where peer was reached
	ip utils.IPDesc

	// expiry is network time when the ip should be released
	expiry time.Time
}

type peer struct {
	net *network // network this peer is part of

	// if the version message has been received and is valid. is only modified
	// on the connection's reader routine.
	gotVersion utils.AtomicBool

	// if the gotPeerList message has been received and is valid. is only
	// modified on the connection's reader routine.
	gotPeerList utils.AtomicBool

	// if the version message has been received and is valid and the peerlist
	// has been returned. is only modified on the connection's reader routine.
	connected utils.AtomicBool

	// if the peer is connected and the peer is able to follow the protocol. is
	// only modified on the connection's reader routine.
	compatible utils.AtomicBool

	// only close the peer once
	once sync.Once

	// if the close function has been called.
	closed utils.AtomicBool

	// number of bytes currently in the send queue.
	pendingBytes int64

	// lock to ensure that closing of the sender queue is handled safely
	senderLock sync.Mutex

	// queue of messages this connection is attempting to send the peer. Is
	// closed when the connection is closed.
	sender chan []byte

	// ip may or may not be set when the peer is first started. is only modified
	// on the connection's reader routine.
	ip utils.IPDesc

	// ipLock must be held when accessing [ip].
	ipLock sync.RWMutex

	// aliases is a list of IPs other than [ip] that we have connected to
	// this peer at.
	aliases []alias

	// aliasTimer triggers the release of expired records from [aliases].
	aliasTimer *timer.Timer

	// aliasLock must be held when accessing [aliases] or [aliasTimer].
	aliasLock sync.Mutex

	// id should be set when the peer is first created.
	id ids.ShortID

	// the connection object that is used to read/write messages from
	conn net.Conn

	// version that the peer reported during the handshake
	versionStruct, versionStr utils.AtomicInterface

	// unix time of the last message sent and received respectively
	// Must only be accessed atomically
	lastSent, lastReceived int64

	tickerCloser chan struct{}

	// ticker processes
	tickerOnce sync.Once
}

// newPeer returns a properly initialized *peer.
func newPeer(net *network, conn net.Conn, ip utils.IPDesc) *peer {
	p := &peer{
		net:          net,
		conn:         conn,
		ip:           ip,
		tickerCloser: make(chan struct{}),
	}
	p.aliasTimer = timer.NewTimer(p.releaseExpiredAliases)

	return p
}

// assume the [stateLock] is held
func (p *peer) Start() {
	go p.ReadMessages()
	go p.WriteMessages()
}

func (p *peer) StartTicker() {
	go p.requestFinishHandshake()
	go p.sendPings()
	go p.monitorAliases()
}

func (p *peer) sendPings() {
	sendPingsTicker := time.NewTicker(p.net.pingFrequency)
	defer sendPingsTicker.Stop()

	for {
		select {
		case <-sendPingsTicker.C:
			closed := p.closed.GetValue()

			if closed {
				return
			}

			p.Ping()
		case <-p.tickerCloser:
			return
		}
	}
}

// request missing handshake messages from the peer
func (p *peer) requestFinishHandshake() {
	finishHandshakeTicker := time.NewTicker(p.net.getVersionTimeout)
	defer finishHandshakeTicker.Stop()

	for {
		select {
		case <-finishHandshakeTicker.C:
			gotVersion := p.gotVersion.GetValue()
			gotPeerList := p.gotPeerList.GetValue()
			connected := p.connected.GetValue()
			closed := p.closed.GetValue()

			if connected || closed {
				return
			}

			if !gotVersion {
				p.GetVersion()
			}
			if !gotPeerList {
				p.GetPeerList()
			}
		case <-p.tickerCloser:
			return
		}
	}
}

// monitorAliases periodically attempts
// to release timed out alias IPs of the
// peer.
//
// monitorAliases will acquire [stateLock]
// when an alias is released.
func (p *peer) monitorAliases() {
	go func() {
		<-p.tickerCloser
		p.aliasTimer.Stop()
	}()

	p.aliasTimer.Dispatch()
}

// attempt to read messages from the peer
func (p *peer) ReadMessages() {
	defer p.Close()

	if err := p.conn.SetReadDeadline(p.net.clock.Time().Add(p.net.pingPongTimeout)); err != nil {
		p.net.log.Verbo("error on setting the connection read timeout %s", err)
		return
	}

	pendingBuffer := wrappers.Packer{}
	readBuffer := make([]byte, p.net.readBufferSize)
	for {
		read, err := p.conn.Read(readBuffer)
		if err != nil {
			p.net.log.Verbo("error on connection read to %s %s %s", p.id, p.getIP(), err)
			return
		}

		pendingBuffer.Bytes = append(pendingBuffer.Bytes, readBuffer[:read]...)

		msgBytes := pendingBuffer.UnpackBytes()
		if pendingBuffer.Errored() {
			// if reading the bytes errored, then we haven't read the full
			// message yet
			pendingBuffer.Offset = 0
			pendingBuffer.Err = nil

			if int64(len(pendingBuffer.Bytes)) > p.net.maxMessageSize+wrappers.IntLen {
				// we have read more bytes than the max message size allows for,
				// so we should terminate this connection

				p.net.log.Verbo("error reading too many bytes on %s %s", p.id, err)
				return
			}

			// we should try to read more bytes to finish the message
			continue
		}

		// we read the full message bytes

		// set the pending bytes to any extra bytes that were read
		pendingBuffer.Bytes = pendingBuffer.Bytes[pendingBuffer.Offset:]
		// set the offset back to the start of the next message
		pendingBuffer.Offset = 0

		if int64(len(msgBytes)) > p.net.maxMessageSize {
			// if this message is longer than the max message length, then we
			// should terminate this connection

			p.net.log.Verbo("error reading too many bytes on %s %s", p.id, err)
			return
		}

		p.net.log.Verbo("parsing new message from %s:\n%s",
			p.id,
			formatting.DumpBytes{Bytes: msgBytes})

		msg, err := p.net.b.Parse(msgBytes)
		if err != nil {
			p.net.log.Debug("failed to parse new message from %s:\n%s\n%s",
				p.id,
				formatting.DumpBytes{Bytes: msgBytes},
				err)
			continue
		}

		p.handle(msg)
	}
}

// attempt to write messages to the peer
func (p *peer) WriteMessages() {
	defer p.Close()

	p.Version()

	for msg := range p.sender {
		p.net.log.Verbo("sending new message to %s:\n%s",
			p.id,
			formatting.DumpBytes{Bytes: msg})

		atomic.AddInt64(&p.pendingBytes, -int64(len(msg)))
		atomic.AddInt64(&p.net.pendingBytes, -int64(len(msg)))

		msgb := [wrappers.IntLen]byte{}
		binary.BigEndian.PutUint32(msgb[:], uint32(len(msg)))
		for _, byteSlice := range [][]byte{msgb[:], msg} {
			for len(byteSlice) > 0 {
				written, err := p.conn.Write(byteSlice)
				if err != nil {
					p.net.log.Verbo("error writing to %s at %s due to: %s", p.id, p.getIP(), err)
					return
				}
				p.tickerOnce.Do(p.StartTicker)
				byteSlice = byteSlice[written:]
			}
		}
		now := p.net.clock.Time().Unix()
		atomic.StoreInt64(&p.lastSent, now)
		atomic.StoreInt64(&p.net.lastMsgSentTime, now)
	}
}

// send assumes that the [stateLock] is not held.
func (p *peer) Send(msg Msg) bool {
	p.senderLock.Lock()
	defer p.senderLock.Unlock()

	// If the peer was closed then the sender channel was closed and we are
	// unable to send this message without panicking. So drop the message.
	if p.closed.GetValue() {
		p.net.log.Debug("dropping message to %s due to a closed connection", p.id)
		return false
	}

	// is it possible to send?
	if dropMsg := p.dropMessagePeer(); dropMsg {
		p.net.log.Debug("dropping message to %s due to a send queue with too many bytes", p.id)
		return false
	}

	msgBytes := msg.Bytes()
	msgBytesLen := int64(len(msgBytes))

	// lets assume send will be successful, we add to the network pending bytes
	// if we determine that we are being a bit restrictive, we could increase the global bandwidth?
	newPendingBytes := atomic.AddInt64(&p.net.pendingBytes, msgBytesLen)

	newConnPendingBytes := atomic.LoadInt64(&p.pendingBytes) + msgBytesLen
	if dropMsg := p.dropMessage(newConnPendingBytes, newPendingBytes); dropMsg {
		// we never sent the message, remove from pending totals
		atomic.AddInt64(&p.net.pendingBytes, -msgBytesLen)
		p.net.log.Debug("dropping message to %s due to a send queue with too many bytes", p.id)
		return false
	}

	select {
	case p.sender <- msgBytes:
		atomic.AddInt64(&p.pendingBytes, msgBytesLen)
		return true
	default:
		// we never sent the message, remove from pending totals
		atomic.AddInt64(&p.net.pendingBytes, -msgBytesLen)
		p.net.log.Debug("dropping message to %s due to a full send queue", p.id)
		return false
	}
}

// assumes the [stateLock] is not held
func (p *peer) handle(msg Msg) {
	now := p.net.clock.Time()
	atomic.StoreInt64(&p.lastReceived, now.Unix())
	atomic.StoreInt64(&p.net.lastMsgReceivedTime, now.Unix())

	if err := p.conn.SetReadDeadline(now.Add(p.net.pingPongTimeout)); err != nil {
		p.net.log.Verbo("error on setting the connection read timeout %s, closing the connection", err)
		p.Close()
		return
	}

	op := msg.Op()
	msgMetrics := p.net.message(op)
	if msgMetrics == nil {
		p.net.log.Debug("dropping an unknown message from %s with op %s", p.id, op.String())
		return
	}
	msgMetrics.numReceived.Inc()
	msgMetrics.receivedBytes.Add(float64(len(msg.Bytes())))

	switch op {
	case Version:
		p.version(msg)
		return
	case GetVersion:
		p.getVersion(msg)
		return
	case Ping:
		p.ping(msg)
		return
	case Pong:
		p.pong(msg)
		return
	case GetPeerList:
		p.getPeerList(msg)
		return
	case PeerList:
		p.peerList(msg)
		return
	}
	if !p.connected.GetValue() {
		p.net.log.Debug("dropping message from %s because the connection hasn't been established yet", p.id)

		// attempt to finish the handshake
		if !p.gotVersion.GetValue() {
			p.GetVersion()
		}
		if !p.gotPeerList.GetValue() {
			p.GetPeerList()
		}
		return
	}

	peerVersion := p.versionStruct.GetValue().(version.Version)
	if p.net.versionCompatibility.Compatible(peerVersion) != nil {
		p.net.log.Verbo("dropping message from un-upgraded validator %s", p.id)

		if p.compatible.GetValue() {
			p.net.stateLock.Lock()
			defer p.net.stateLock.Unlock()

			if p.compatible.GetValue() {
				p.net.router.Disconnected(p.id)
				p.compatible.SetValue(false)
			}
		}
		return
	}

	switch op {
	case GetAcceptedFrontier:
		p.getAcceptedFrontier(msg)
	case AcceptedFrontier:
		p.acceptedFrontier(msg)
	case GetAccepted:
		p.getAccepted(msg)
	case Accepted:
		p.accepted(msg)
	case Get:
		p.get(msg)
	case GetAncestors:
		p.getAncestors(msg)
	case Put:
		p.put(msg)
	case MultiPut:
		p.multiPut(msg)
	case PushQuery:
		p.pushQuery(msg)
	case PullQuery:
		p.pullQuery(msg)
	case Chits:
		p.chits(msg)
	default:
		p.net.log.Debug("dropping an unknown message from %s with op %s", p.id, op.String())
	}
}

func (p *peer) dropMessagePeer() bool {
	return atomic.LoadInt64(&p.pendingBytes) > p.net.maxMessageSize
}

func (p *peer) dropMessage(connPendingLen, networkPendingLen int64) bool {
	return networkPendingLen > p.net.networkPendingSendBytesToRateLimit && // Check to see if we should be enforcing any rate limiting
		p.dropMessagePeer() && // this connection should have a minimum allowed bandwidth
		(networkPendingLen > p.net.maxNetworkPendingSendBytes || // Check to see if this message would put too much memory into the network
			connPendingLen > p.net.maxNetworkPendingSendBytes/20) // Check to see if this connection is using too much memory
}

// assumes the [stateLock] is not held
func (p *peer) Close() { p.once.Do(p.close) }

// assumes only `peer.Close` calls this
func (p *peer) close() {
	// If the connection is closing, we can immediately cancel the ticker
	// goroutines.
	close(p.tickerCloser)

	p.closed.SetValue(true)

	if err := p.conn.Close(); err != nil {
		p.net.log.Debug("closing peer %s resulted in an error: %s", p.id, err)
	}

	p.senderLock.Lock()
	// The locks guarantee here that the sender routine will read that the peer
	// has been closed and will therefore not attempt to write on this channel.
	close(p.sender)
	p.senderLock.Unlock()

	peerPending := atomic.LoadInt64(&p.pendingBytes)
	atomic.AddInt64(&p.net.pendingBytes, -peerPending)

	p.net.disconnected(p)
}

// assumes the [stateLock] is not held
func (p *peer) GetVersion() {
	msg, err := p.net.b.GetVersion()
	p.net.log.AssertNoError(err)
	if p.Send(msg) {
		p.net.getVersion.numSent.Inc()
		p.net.getVersion.sentBytes.Add(float64(len(msg.Bytes())))
		p.net.sendFailRateCalculator.Observe(0, p.net.clock.Time())
	} else {
		p.net.getVersion.numFailed.Inc()
		p.net.sendFailRateCalculator.Observe(1, p.net.clock.Time())
	}
}

// assumes the [stateLock] is not held
func (p *peer) Version() {
	p.net.stateLock.RLock()
	msg, err := p.net.b.Version(
		p.net.networkID,
		p.net.nodeID,
		p.net.clock.Unix(),
		p.net.ip.IP(),
		p.net.versionCompatibility.Version().String(),
	)
	p.net.stateLock.RUnlock()
	p.net.log.AssertNoError(err)
	if p.Send(msg) {
		p.net.metrics.version.numSent.Inc()
		p.net.metrics.version.sentBytes.Add(float64(len(msg.Bytes())))
		p.net.sendFailRateCalculator.Observe(0, p.net.clock.Time())
	} else {
		p.net.metrics.version.numFailed.Inc()
		p.net.sendFailRateCalculator.Observe(1, p.net.clock.Time())
	}
}

// assumes the [stateLock] is not held
func (p *peer) GetPeerList() {
	msg, err := p.net.b.GetPeerList()
	p.net.log.AssertNoError(err)
	if p.Send(msg) {
		p.net.getPeerlist.numSent.Inc()
		p.net.getPeerlist.sentBytes.Add(float64(len(msg.Bytes())))
		p.net.sendFailRateCalculator.Observe(0, p.net.clock.Time())
	} else {
		p.net.getPeerlist.numFailed.Inc()
		p.net.sendFailRateCalculator.Observe(1, p.net.clock.Time())
	}
}

// assumes the stateLock is not held
func (p *peer) SendPeerList() {
	ips := p.net.validatorIPs()
	p.PeerList(ips)
}

// assumes the [stateLock] is not held
func (p *peer) PeerList(peers []utils.IPDesc) {
	msg, err := p.net.b.PeerList(peers)
	if err != nil {
		p.net.log.Warn("failed to send PeerList message due to %s", err)
		return
	}
	if p.Send(msg) {
		p.net.peerlist.numSent.Inc()
		p.net.peerlist.sentBytes.Add(float64(len(msg.Bytes())))
		p.net.sendFailRateCalculator.Observe(0, p.net.clock.Time())
	} else {
		p.net.peerlist.numFailed.Inc()
		p.net.sendFailRateCalculator.Observe(1, p.net.clock.Time())
	}
}

// assumes the [stateLock] is not held
func (p *peer) Ping() {
	msg, err := p.net.b.Ping()
	p.net.log.AssertNoError(err)
	if p.Send(msg) {
		p.net.ping.numSent.Inc()
		p.net.ping.sentBytes.Add(float64(len(msg.Bytes())))
		p.net.sendFailRateCalculator.Observe(0, p.net.clock.Time())
	} else {
		p.net.ping.numFailed.Inc()
		p.net.sendFailRateCalculator.Observe(1, p.net.clock.Time())
	}
}

// assumes the [stateLock] is not held
func (p *peer) Pong() {
	msg, err := p.net.b.Pong()
	p.net.log.AssertNoError(err)
	if p.Send(msg) {
		p.net.pong.numSent.Inc()
		p.net.pong.sentBytes.Add(float64(len(msg.Bytes())))
		p.net.sendFailRateCalculator.Observe(0, p.net.clock.Time())
	} else {
		p.net.pong.numFailed.Inc()
		p.net.sendFailRateCalculator.Observe(1, p.net.clock.Time())
	}
}

// assumes the [stateLock] is not held
func (p *peer) getVersion(msg Msg) {
	p.Version()
}

// assumes the [stateLock] is not held
func (p *peer) version(msg Msg) {
	if p.gotVersion.GetValue() {
		p.net.log.Verbo("dropping duplicated version message from %s", p.id)
		return
	}

	if networkID := msg.Get(NetworkID).(uint32); networkID != p.net.networkID {
		p.net.log.Debug("peer's network ID doesn't match our networkID: Peer's = %d ; Ours = %d",
			networkID,
			p.net.networkID)

		p.discardIP()
		return
	}

	if nodeID := msg.Get(NodeID).(uint32); nodeID == p.net.nodeID {
		p.net.log.Debug("peer's node ID matches our nodeID")

		p.discardMyIP()
		return
	}

	myTime := float64(p.net.clock.Unix())
	if peerTime := float64(msg.Get(MyTime).(uint64)); math.Abs(peerTime-myTime) > p.net.maxClockDifference.Seconds() {
		if p.net.beacons.Contains(p.id) {
			p.net.log.Warn("beacon %s has a clock that is too far out of sync with mine. Peer's = %d, Ours = %d (seconds)",
				p.id,
				uint64(peerTime),
				uint64(myTime))
		} else {
			p.net.log.Debug("peer %s has a clock that is too far out of sync with mine. Peer's = %d, Ours = %d (seconds)",
				p.id,
				uint64(peerTime),
				uint64(myTime))
		}

		p.discardIP()
		return
	}

	peerVersionStr := msg.Get(VersionStr).(string)
	peerVersion, err := p.net.parser.Parse(peerVersionStr)
	if err != nil {
		p.net.log.Debug("peer version could not be parsed due to %s", err)

		p.discardIP()
		return
	}

	if p.net.versionCompatibility.Version().Before(peerVersion) {
		if p.net.beacons.Contains(p.id) {
			p.net.log.Info("beacon %s attempting to connect with newer version %s. You may want to update your client",
				p.id,
				peerVersion)
		} else {
			p.net.log.Debug("peer %s attempting to connect with newer version %s. You may want to update your client",
				p.id,
				peerVersion)
		}
	}

	if err := p.net.versionCompatibility.Connectable(peerVersion); err != nil {
		p.net.log.Debug("peer version not compatible due to %s", err)

		if !p.net.beacons.Contains(p.id) {
			p.discardIP()
			return
		}
		p.net.log.Info("allowing beacon %s to connect with a lower version %s",
			p.id,
			peerVersion)
	}

	ip := p.getIP()
	if ip.IsZero() {
		// we only care about the claimed IP if we don't know the IP yet
		peerIP := msg.Get(IP).(utils.IPDesc)

		addr := p.conn.RemoteAddr()
		localPeerIP, err := utils.ToIPDesc(addr.String())
		if err == nil {
			// If we have no clue what the peer's IP is, we can't perform any
			// verification
			if peerIP.IP.Equal(localPeerIP.IP) {
				// if the IPs match, add this ip:port pair to be tracked
				p.setIP(peerIP)
			}
		}
	}

	p.SendPeerList()

	p.versionStruct.SetValue(peerVersion)
	p.versionStr.SetValue(peerVersion.String())
	p.gotVersion.SetValue(true)

	p.tryMarkConnected()
}

// assumes the [stateLock] is not held
func (p *peer) getPeerList(msg Msg) {
	if p.gotVersion.GetValue() {
		p.SendPeerList()
	}
}

// assumes the [stateLock] is not held
func (p *peer) peerList(msg Msg) {
	ips := msg.Get(Peers).([]utils.IPDesc)

	p.gotPeerList.SetValue(true)
	p.tryMarkConnected()

	for _, ip := range ips {
		p.net.stateLock.Lock()
		if !ip.Equal(p.net.ip.IP()) &&
			!ip.IsZero() &&
			(p.net.allowPrivateIPs || !ip.IsPrivate()) {
			// TODO: only try to connect once
			p.net.track(ip)
		}
		p.net.stateLock.Unlock()
	}
}

// assumes the [stateLock] is not held
func (p *peer) ping(_ Msg) {
	p.Pong()
}

// assumes the [stateLock] is not held
func (p *peer) pong(_ Msg) {}

// assumes the [stateLock] is not held
func (p *peer) getAcceptedFrontier(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	deadline := p.net.clock.Time().Add(time.Duration(msg.Get(Deadline).(uint64)))

	p.net.router.GetAcceptedFrontier(p.id, chainID, requestID, deadline)
}

// assumes the [stateLock] is not held
func (p *peer) acceptedFrontier(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)

	containerIDsBytes := msg.Get(ContainerIDs).([][]byte)
	containerIDs := make([]ids.ID, len(containerIDsBytes))
	containerIDsSet := ids.Set{} // To prevent duplicates
	for i, containerIDBytes := range containerIDsBytes {
		containerID, err := ids.ToID(containerIDBytes)
		if err != nil {
			p.net.log.Debug("error parsing ContainerID 0x%x: %s", containerIDBytes, err)
			return
		}
		if containerIDsSet.Contains(containerID) {
			p.net.log.Debug("message contains duplicate of container ID %s", containerID)
			return
		}
		containerIDs[i] = containerID
		containerIDsSet.Add(containerID)
	}

	p.net.router.AcceptedFrontier(p.id, chainID, requestID, containerIDs)
}

// assumes the [stateLock] is not held
func (p *peer) getAccepted(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	deadline := p.net.clock.Time().Add(time.Duration(msg.Get(Deadline).(uint64)))

	containerIDsBytes := msg.Get(ContainerIDs).([][]byte)
	containerIDs := make([]ids.ID, len(containerIDsBytes))
	containerIDsSet := ids.Set{} // To prevent duplicates
	for i, containerIDBytes := range containerIDsBytes {
		containerID, err := ids.ToID(containerIDBytes)
		if err != nil {
			p.net.log.Debug("error parsing ContainerID 0x%x: %s", containerIDBytes, err)
			return
		}
		if containerIDsSet.Contains(containerID) {
			p.net.log.Debug("message contains duplicate of container ID %s", containerID)
			return
		}
		containerIDs[i] = containerID
		containerIDsSet.Add(containerID)
	}

	p.net.router.GetAccepted(p.id, chainID, requestID, deadline, containerIDs)
}

// assumes the [stateLock] is not held
func (p *peer) accepted(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)

	containerIDsBytes := msg.Get(ContainerIDs).([][]byte)
	containerIDs := make([]ids.ID, len(containerIDsBytes))
	containerIDsSet := ids.Set{} // To prevent duplicates
	for i, containerIDBytes := range containerIDsBytes {
		containerID, err := ids.ToID(containerIDBytes)
		if err != nil {
			p.net.log.Debug("error parsing ContainerID 0x%x: %s", containerIDBytes, err)
			return
		}
		if containerIDsSet.Contains(containerID) {
			p.net.log.Debug("message contains duplicate of container ID %s", containerID)
			return
		}
		containerIDs[i] = containerID
		containerIDsSet.Add(containerID)
	}

	p.net.router.Accepted(p.id, chainID, requestID, containerIDs)
}

// assumes the [stateLock] is not held
func (p *peer) get(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	deadline := p.net.clock.Time().Add(time.Duration(msg.Get(Deadline).(uint64)))
	containerID, err := ids.ToID(msg.Get(ContainerID).([]byte))
	p.net.log.AssertNoError(err)

	p.net.router.Get(p.id, chainID, requestID, deadline, containerID)
}

func (p *peer) getAncestors(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	deadline := p.net.clock.Time().Add(time.Duration(msg.Get(Deadline).(uint64)))
	containerID, err := ids.ToID(msg.Get(ContainerID).([]byte))
	p.net.log.AssertNoError(err)

	p.net.router.GetAncestors(p.id, chainID, requestID, deadline, containerID)
}

// assumes the [stateLock] is not held
func (p *peer) put(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	containerID, err := ids.ToID(msg.Get(ContainerID).([]byte))
	p.net.log.AssertNoError(err)
	container := msg.Get(ContainerBytes).([]byte)

	p.net.router.Put(p.id, chainID, requestID, containerID, container)
}

// assumes the [stateLock] is not held
func (p *peer) multiPut(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	containers := msg.Get(MultiContainerBytes).([][]byte)

	p.net.router.MultiPut(p.id, chainID, requestID, containers)
}

// assumes the [stateLock] is not held
func (p *peer) pushQuery(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	deadline := p.net.clock.Time().Add(time.Duration(msg.Get(Deadline).(uint64)))
	containerID, err := ids.ToID(msg.Get(ContainerID).([]byte))
	p.net.log.AssertNoError(err)
	container := msg.Get(ContainerBytes).([]byte)

	p.net.router.PushQuery(p.id, chainID, requestID, deadline, containerID, container)
}

// assumes the [stateLock] is not held
func (p *peer) pullQuery(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)
	deadline := p.net.clock.Time().Add(time.Duration(msg.Get(Deadline).(uint64)))
	containerID, err := ids.ToID(msg.Get(ContainerID).([]byte))
	p.net.log.AssertNoError(err)

	p.net.router.PullQuery(p.id, chainID, requestID, deadline, containerID)
}

// assumes the [stateLock] is not held
func (p *peer) chits(msg Msg) {
	chainID, err := ids.ToID(msg.Get(ChainID).([]byte))
	p.net.log.AssertNoError(err)
	requestID := msg.Get(RequestID).(uint32)

	containerIDsBytes := msg.Get(ContainerIDs).([][]byte)
	containerIDs := make([]ids.ID, len(containerIDsBytes))
	containerIDsSet := ids.Set{} // To prevent duplicates
	for i, containerIDBytes := range containerIDsBytes {
		containerID, err := ids.ToID(containerIDBytes)
		if err != nil {
			p.net.log.Debug("error parsing ContainerID 0x%x: %s", containerIDBytes, err)
			return
		}
		if containerIDsSet.Contains(containerID) {
			p.net.log.Debug("message contains duplicate of container ID %s", containerID)
			return
		}
		containerIDs[i] = containerID
		containerIDsSet.Add(containerID)
	}

	p.net.router.Chits(p.id, chainID, requestID, containerIDs)
}

// assumes the [stateLock] is held
func (p *peer) tryMarkConnected() {
	if !p.connected.GetValue() && // not already connected
		p.gotVersion.GetValue() && // not waiting for version
		p.gotPeerList.GetValue() && // not waiting for peerlist
		!p.closed.GetValue() { // and not already disconnected
		p.net.connected(p)
	}
}

func (p *peer) discardIP() {
	// By clearing the IP, we will not attempt to reconnect to this peer
	if ip := p.getIP(); !ip.IsZero() {
		p.setIP(utils.IPDesc{})

		p.net.stateLock.Lock()
		delete(p.net.disconnectedIPs, ip.String())
		p.net.stateLock.Unlock()
	}
	p.Close()
}

func (p *peer) discardMyIP() {
	// By clearing the IP, we will not attempt to reconnect to this peer
	if ip := p.getIP(); !ip.IsZero() {
		p.setIP(utils.IPDesc{})

		str := ip.String()

		p.net.stateLock.Lock()
		p.net.myIPs[str] = struct{}{}
		delete(p.net.disconnectedIPs, str)
		p.net.stateLock.Unlock()
	}
	p.Close()
}

func (p *peer) setIP(ip utils.IPDesc) {
	p.ipLock.Lock()
	defer p.ipLock.Unlock()
	p.ip = ip
}

func (p *peer) getIP() utils.IPDesc {
	p.ipLock.RLock()
	defer p.ipLock.RUnlock()
	return p.ip
}

// addAlias marks that we have found another
// IP that we can connect to this peer at.
//
// assumes [stateLock] is held
func (p *peer) addAlias(ip utils.IPDesc) {
	p.aliasLock.Lock()
	defer p.aliasLock.Unlock()

	p.net.peerAliasIPs[ip.String()] = struct{}{}
	p.aliases = append(p.aliases, alias{
		ip:     ip,
		expiry: p.net.clock.Time().Add(p.net.peerAliasTimeout),
	})

	// Set the [aliasTimer] if this ip is the first alias we put
	// in [aliases].
	if len(p.aliases) == 1 {
		p.aliasTimer.SetTimeoutIn(p.net.peerAliasTimeout)
	}
}

// releaseNextAlias returns the next released alias or nil if none was released.
// If none was released, then this will schedule the next time to remove an
// alias.
//
// assumes [stateLock] is held
func (p *peer) releaseNextAlias(now time.Time) *alias {
	p.aliasLock.Lock()
	defer p.aliasLock.Unlock()

	if len(p.aliases) == 0 {
		return nil
	}

	next := p.aliases[0]
	if timeUntilExpiry := next.expiry.Sub(now); timeUntilExpiry > 0 {
		p.aliasTimer.SetTimeoutIn(timeUntilExpiry)
		return nil
	}
	p.aliases = p.aliases[1:]

	p.net.log.Verbo("released alias %s for peer %s", next.ip, p.id)
	return &next
}

// releaseExpiredAliases frees expired IP aliases. If there is an IP pending
// expiration, then the expiration is scheduled.
//
// assumes [stateLock] is not held
func (p *peer) releaseExpiredAliases() {
	currentTime := p.net.clock.Time()
	for {
		next := p.releaseNextAlias(currentTime)
		if next == nil {
			return
		}

		// We should always release [aliasLock] before attempting
		// to acquire the [stateLock] to avoid deadlocking on addAlias.
		p.net.stateLock.Lock()
		delete(p.net.peerAliasIPs, next.ip.String())
		p.net.stateLock.Unlock()
	}
}

// releaseAllAliases frees all alias IPs.
//
// assumes [stateLock] is held and that [aliasTimer]
// has been stopped
func (p *peer) releaseAllAliases() {
	p.aliasLock.Lock()
	defer p.aliasLock.Unlock()

	for _, alias := range p.aliases {
		delete(p.net.peerAliasIPs, alias.ip.String())

		p.net.log.Verbo("released alias %s for peer %s", alias.ip, p.id)
	}
	p.aliases = nil
}
