// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/networking/benchlist"
	"github.com/ava-labs/avalanchego/snow/networking/router"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/version"
)

const (
	defaultSendQueueSize = 1 << 10
	defaultAliasTimeout  = 2 * time.Second
)

var (
	errClosed  = errors.New("closed")
	errRefused = errors.New("connection refused")
)

type testListener struct {
	addr    net.Addr
	inbound chan net.Conn
	once    sync.Once
	closed  chan struct{}
}

func (l *testListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.inbound:
		return c, nil
	case <-l.closed:
		return nil, errClosed
	}
}

func (l *testListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *testListener) Addr() net.Addr { return l.addr }

type testDialer struct {
	addr net.Addr

	outbounds     map[string]*testListener
	outboundsLock sync.Mutex

	// clients tracks opened client connections
	// by IP
	clients map[string]*testConn

	// closer is invoked when testDialer is closed
	closer func(net.Addr, net.Addr)
}

func (d *testDialer) Dial(ip utils.IPDesc) (net.Conn, error) {
	d.outboundsLock.Lock()
	defer d.outboundsLock.Unlock()

	outbound, ok := d.outbounds[ip.String()]
	if !ok {
		return nil, errRefused
	}
	server := &testConn{
		pendingReads:  make(chan []byte, 1<<10),
		pendingWrites: make(chan []byte, 1<<10),
		closed:        make(chan struct{}),
		local:         outbound.addr,
		remote:        d.addr,
		closer:        d.closer,
	}
	client := &testConn{
		pendingReads:  server.pendingWrites,
		pendingWrites: server.pendingReads,
		closed:        make(chan struct{}),
		local:         d.addr,
		remote:        outbound.addr,
		closer:        d.closer,
	}

	select {
	case outbound.inbound <- server:
		if d.clients == nil {
			d.clients = make(map[string]*testConn)
		}
		d.clients[ip.String()] = client
		return client, nil
	default:
		return nil, errRefused
	}
}

func (d *testDialer) Update(ip utils.DynamicIPDesc, listener *testListener) {
	d.outboundsLock.Lock()
	defer d.outboundsLock.Unlock()

	d.outbounds[ip.String()] = listener
}

type testConn struct {
	partialRead   []byte
	pendingReads  chan []byte
	pendingWrites chan []byte
	closed        chan struct{}
	once          sync.Once

	// closer is invoked when testConn is closed
	closer func(net.Addr, net.Addr)

	local, remote net.Addr
}

func (c *testConn) Read(b []byte) (int, error) {
	for len(c.partialRead) == 0 {
		select {
		case read, ok := <-c.pendingReads:
			if !ok {
				return 0, errClosed
			}
			c.partialRead = read
		case <-c.closed:
			return 0, errClosed
		}
	}

	copy(b, c.partialRead)
	if length := len(c.partialRead); len(b) > length {
		c.partialRead = nil
		return length, nil
	}
	c.partialRead = c.partialRead[len(b):]
	return len(b), nil
}

func (c *testConn) Write(b []byte) (int, error) {
	newB := make([]byte, len(b))
	copy(newB, b)

	select {
	case c.pendingWrites <- newB:
	case <-c.closed:
		return 0, errClosed
	}

	return len(b), nil
}

func (c *testConn) Close() error {
	c.once.Do(func() {
		close(c.closed)

		if c.closer != nil {
			c.closer(c.local, c.remote)
		}
	})
	return nil
}

func (c *testConn) LocalAddr() net.Addr              { return c.local }
func (c *testConn) RemoteAddr() net.Addr             { return c.remote }
func (c *testConn) SetDeadline(time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(time.Time) error { return nil }

type testHandler struct {
	router.Router
	connected    func(ids.ShortID)
	disconnected func(ids.ShortID)
}

func (h *testHandler) Connected(id ids.ShortID) {
	if h.connected != nil {
		h.connected(id)
	}
}
func (h *testHandler) Disconnected(id ids.ShortID) {
	if h.disconnected != nil {
		h.disconnected(id)
	}
}

// testUpgrader is used to provide a custom
// id for a given IP
type testUpgrader struct {
	// ids is a mapping of IP addresses
	// to id
	ids     map[string]ids.ShortID
	idsLock sync.Mutex
}

func (u *testUpgrader) Upgrade(conn net.Conn) (ids.ShortID, net.Conn, error) {
	u.idsLock.Lock()
	defer u.idsLock.Unlock()
	addr := conn.RemoteAddr()
	str := addr.String()
	return u.ids[str], conn, nil
}

func (u *testUpgrader) Update(ip utils.DynamicIPDesc, id ids.ShortID) {
	u.idsLock.Lock()
	defer u.idsLock.Unlock()

	u.ids[ip.String()] = id
}

func TestNewDefaultNetwork(t *testing.T) {
	log := logging.NoLog{}
	ip := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id := ids.ShortID(hashing.ComputeHash160Array([]byte(ip.IP().String())))
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	listener := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	serverUpgrader := NewIPUpgrader()
	clientUpgrader := NewIPUpgrader()

	vdrs := validators.NewSet()
	handler := &testHandler{}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id,
		ip,
		networkID,
		versionManager,
		versionParser,
		listener,
		caller,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net)

	go func() {
		err := net.Close()
		assert.NoError(t, err)
	}()

	err := net.Dispatch()
	assert.Error(t, err)
}

func TestEstablishConnection(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller1.outbounds[ip0.IP().String()] = listener0

	serverUpgrader := NewIPUpgrader()
	clientUpgrader := NewIPUpgrader()

	vdrs := validators.NewSet()

	var (
		wg0 sync.WaitGroup
		wg1 sync.WaitGroup
	)
	wg0.Add(1)
	wg1.Add(1)

	handler0 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id0 {
				wg0.Done()
			}
		},
	}

	handler1 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id1 {
				wg1.Done()
			}
		},
	}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler0,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler1,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()

	net0.Track(ip1.IP())

	wg0.Wait()
	wg1.Wait()

	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)
}

func TestDoubleTrack(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller1.outbounds[ip0.IP().String()] = listener0

	serverUpgrader := NewIPUpgrader()
	clientUpgrader := NewIPUpgrader()

	vdrs := validators.NewSet()

	var (
		wg0 sync.WaitGroup
		wg1 sync.WaitGroup
	)
	wg0.Add(1)
	wg1.Add(1)

	handler0 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id0 {
				wg0.Done()
			}
		},
	}

	handler1 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id1 {
				wg1.Done()
			}
		},
	}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler0,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler1,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	net0.Track(ip1.IP())
	net0.Track(ip1.IP())

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()

	wg0.Wait()
	wg1.Wait()

	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)
}

func TestDoubleClose(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller1.outbounds[ip0.IP().String()] = listener0

	serverUpgrader := NewIPUpgrader()
	clientUpgrader := NewIPUpgrader()

	vdrs := validators.NewSet()

	var (
		wg0 sync.WaitGroup
		wg1 sync.WaitGroup
	)
	wg0.Add(1)
	wg1.Add(1)

	handler0 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id0 {
				wg0.Done()
			}
		},
	}

	handler1 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id1 {
				wg1.Done()
			}
		},
	}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler0,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler1,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	net0.Track(ip1.IP())

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()

	wg0.Wait()
	wg1.Wait()

	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)

	err = net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)
}

func TestTrackConnected(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller1.outbounds[ip0.IP().String()] = listener0

	serverUpgrader := NewIPUpgrader()
	clientUpgrader := NewIPUpgrader()

	vdrs := validators.NewSet()

	var (
		wg0 sync.WaitGroup
		wg1 sync.WaitGroup
	)
	wg0.Add(1)
	wg1.Add(1)

	handler0 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id0 {
				wg0.Done()
			}
		},
	}

	handler1 := &testHandler{
		connected: func(id ids.ShortID) {
			if id != id1 {
				wg1.Done()
			}
		},
	}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler0,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler1,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	net0.Track(ip1.IP())

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()

	wg0.Wait()
	wg1.Wait()

	net0.Track(ip1.IP())

	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)
}

func TestTrackConnectedRace(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller1.outbounds[ip0.IP().String()] = listener0

	serverUpgrader := NewIPUpgrader()
	clientUpgrader := NewIPUpgrader()

	vdrs := validators.NewSet()
	handler := &testHandler{}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	net0.Track(ip1.IP())

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()

	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)
}

func assertEqualPeers(t *testing.T, expected map[string]ids.ShortID, actual []PeerID) {
	assert.Len(t, actual, len(expected))
	for _, p := range actual {
		match, ok := expected[p.IP]
		assert.True(t, ok, "peer with IP %s missing", p.IP)
		assert.Equal(t, match.PrefixedString(constants.NodeIDPrefix), p.ID)
	}
}

func TestPeerAliasesTicker(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))
	ip2 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		2,
	)
	id2 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip2.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}
	listener2 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller2 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		outbounds: make(map[string]*testListener),
	}
	listener3 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller3 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller0.outbounds[ip2.IP().String()] = listener2
	caller1.outbounds[ip0.IP().String()] = listener0
	caller2.outbounds[ip0.IP().String()] = listener0
	caller3.outbounds[ip0.IP().String()] = listener0

	upgrader := &testUpgrader{
		ids: map[string]ids.ShortID{
			ip0.IP().String(): id0,
			ip1.IP().String(): id1,
			ip2.IP().String(): id1,
		},
	}
	serverUpgrader := upgrader
	clientUpgrader := upgrader

	vdrs := validators.NewSet()

	var (
		wg0     sync.WaitGroup
		wg1     sync.WaitGroup
		wg1Done bool
		wg2     sync.WaitGroup

		cleanup bool
	)
	wg0.Add(2)
	wg1.Add(1)
	wg2.Add(2)

	handler0 := &testHandler{
		connected: func(id ids.ShortID) {
			if id == id1 {
				wg0.Done()
				return
			}
			if id == id2 {
				wg2.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 0 unauthorized connection", id.String())
		},
	}

	handler1 := &testHandler{
		connected: func(id ids.ShortID) {
			if id == id0 {
				wg0.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 1 unauthorized connection", id.String())
		},
	}

	handler2 := &testHandler{
		connected: func(id ids.ShortID) {
			if cleanup {
				return
			}

			assert.Fail(t, "handler 2 unauthorized connection", id.String())
		},
	}

	handler3 := &testHandler{
		connected: func(id ids.ShortID) {
			if id == id0 {
				wg2.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 3 unauthorized connection", id.String())
		},
	}

	caller0.closer = func(local net.Addr, remote net.Addr) {
		if remote.String() == ip2.String() && !wg1Done {
			wg1.Done()
			return
		}
		if cleanup {
			return
		}

		assert.Fail(t, "caller 0 unauthorized close", local.String(), remote.String())
	}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler0,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler1,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	net2 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip2,
		networkID,
		versionManager,
		versionParser,
		listener2,
		caller2,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler2,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net2)

	net3 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id2,
		ip2,
		networkID,
		versionManager,
		versionParser,
		listener3,
		caller3,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler3,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net3)

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net2.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net3.Dispatch()
		assert.Error(t, err)
	}()

	// Connect to peer with id1
	net0.Track(ip1.IP())

	// Confirm peers correct
	wg0.Wait()
	assertEqualPeers(t, map[string]ids.ShortID{
		ip1.String(): id1,
	}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assert.Len(t, net3.Peers([]ids.ShortID{}), 0)

	// Attempt to connect to ip2 (same id as ip1)
	net0.Track(ip2.IP())

	// Confirm that ip2 was not added to net0 peers
	wg1.Wait()
	wg1Done = true
	assertEqualPeers(t, map[string]ids.ShortID{
		ip1.String(): id1,
	}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assert.Len(t, net3.Peers([]ids.ShortID{}), 0)

	// Subsequent track call returns immediately with no connection attempts
	// (would cause fatal error from unauthorized connection if allowed)
	net0.Track(ip2.IP())

	// Wait for aliases to be removed by peer
	time.Sleep(3 * time.Second)

	// Track ip2 on net3
	upgrader.Update(ip2, id2)
	caller0.Update(ip2, listener3)
	net0.Track(ip2.IP())

	// Confirm that id2 was added as peer
	wg2.Wait()
	assertEqualPeers(t, map[string]ids.ShortID{
		ip1.String(): id1,
		ip2.String(): id2,
	}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net3.Peers([]ids.ShortID{}))

	// Cleanup
	cleanup = true
	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)

	err = net2.Close()
	assert.NoError(t, err)

	err = net3.Close()
	assert.NoError(t, err)
}

func TestPeerAliasesDisconnect(t *testing.T) {
	log := logging.NoLog{}
	networkID := uint32(0)
	appVersion := version.NewDefaultVersion("app", 0, 1, 0)
	versionParser := version.NewDefaultParser()

	ip0 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		0,
	)
	id0 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip0.IP().String())))
	ip1 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		1,
	)
	id1 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip1.IP().String())))
	ip2 := utils.NewDynamicIPDesc(
		net.IPv6loopback,
		2,
	)
	id2 := ids.ShortID(hashing.ComputeHash160Array([]byte(ip2.IP().String())))

	listener0 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller0 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 0,
		},
		outbounds: make(map[string]*testListener),
	}
	listener1 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller1 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 1,
		},
		outbounds: make(map[string]*testListener),
	}
	listener2 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller2 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		outbounds: make(map[string]*testListener),
	}
	listener3 := &testListener{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		inbound: make(chan net.Conn, 1<<10),
		closed:  make(chan struct{}),
	}
	caller3 := &testDialer{
		addr: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: 2,
		},
		outbounds: make(map[string]*testListener),
	}

	caller0.outbounds[ip1.IP().String()] = listener1
	caller0.outbounds[ip2.IP().String()] = listener2
	caller1.outbounds[ip0.IP().String()] = listener0
	caller2.outbounds[ip0.IP().String()] = listener0
	caller3.outbounds[ip0.IP().String()] = listener0

	upgrader := &testUpgrader{
		ids: map[string]ids.ShortID{
			ip0.IP().String(): id0,
			ip1.IP().String(): id1,
			ip2.IP().String(): id1,
		},
	}
	serverUpgrader := upgrader
	clientUpgrader := upgrader

	vdrs := validators.NewSet()

	var (
		wg0     sync.WaitGroup
		wg1     sync.WaitGroup
		wg1Done bool
		wg2     sync.WaitGroup
		wg2Done bool
		wg3     sync.WaitGroup

		cleanup bool
	)
	wg0.Add(2)
	wg1.Add(1)
	wg2.Add(3)
	wg3.Add(2)

	handler0 := &testHandler{
		connected: func(id ids.ShortID) {
			if id == id1 {
				wg0.Done()
				return
			}
			if id == id2 {
				wg3.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 0 unauthorized connection", id.String())
		},
		disconnected: func(id ids.ShortID) {
			if id == id1 {
				wg2.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 0 unauthorized disconnect", id.String())
		},
	}

	handler1 := &testHandler{
		connected: func(id ids.ShortID) {
			if id == id0 {
				wg0.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 1 unauthorized connection", id.String())
		},
	}

	handler2 := &testHandler{
		connected: func(id ids.ShortID) {
			if cleanup {
				return
			}

			assert.Fail(t, "handler 2 unauthorized connection", id.String())
		},
	}

	handler3 := &testHandler{
		connected: func(id ids.ShortID) {
			if id == id0 {
				wg3.Done()
				return
			}
			if cleanup {
				return
			}

			assert.Fail(t, "handler 3 unauthorized connection", id.String())
		},
	}

	caller0.closer = func(local net.Addr, remote net.Addr) {
		if remote.String() == ip2.String() && !wg1Done {
			wg1.Done()
			return
		}
		if remote.String() == ip1.String() && !wg2Done {
			wg2.Done()
			return
		}
		if local.String() == ip1.String() && !wg2Done {
			wg2.Done()
			return
		}
		if cleanup {
			return
		}

		assert.Fail(t, "caller 0 unauthorized close", local.String(), remote.String())
	}

	versionManager := version.NewCompatibility(
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
		appVersion,
		time.Now(),
		appVersion,
	)

	net0 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id0,
		ip0,
		networkID,
		versionManager,
		versionParser,
		listener0,
		caller0,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler0,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net0)

	net1 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip1,
		networkID,
		versionManager,
		versionParser,
		listener1,
		caller1,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler1,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net1)

	net2 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id1,
		ip2,
		networkID,
		versionManager,
		versionParser,
		listener2,
		caller2,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler2,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net2)

	net3 := NewDefaultNetwork(
		prometheus.NewRegistry(),
		log,
		id2,
		ip2,
		networkID,
		versionManager,
		versionParser,
		listener3,
		caller3,
		serverUpgrader,
		clientUpgrader,
		vdrs,
		vdrs,
		handler3,
		time.Duration(0),
		0,
		nil,
		false,
		0,
		0,
		defaultSendQueueSize,
		HealthConfig{},
		benchlist.NewManager(&benchlist.Config{}),
		defaultAliasTimeout,
	)
	assert.NotNil(t, net3)

	go func() {
		err := net0.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net1.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net2.Dispatch()
		assert.Error(t, err)
	}()
	go func() {
		err := net3.Dispatch()
		assert.Error(t, err)
	}()

	// Connect to peer with id1
	net0.Track(ip1.IP())

	// Confirm peers correct
	wg0.Wait()
	assertEqualPeers(t, map[string]ids.ShortID{
		ip1.String(): id1,
	}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assert.Len(t, net3.Peers([]ids.ShortID{}), 0)

	// Attempt to connect to ip2 (same id as ip1)
	net0.Track(ip2.IP())

	// Confirm that ip2 was not added to net0 peers
	wg1.Wait()
	wg1Done = true
	assertEqualPeers(t, map[string]ids.ShortID{
		ip1.String(): id1,
	}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assert.Len(t, net3.Peers([]ids.ShortID{}), 0)

	// Disconnect original peer
	caller0.clients[ip1.String()].Close()

	// Track ip2 on net3
	wg2.Wait()
	wg2Done = true
	assertEqualPeers(t, map[string]ids.ShortID{}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assert.Len(t, net3.Peers([]ids.ShortID{}), 0)
	upgrader.Update(ip2, id2)
	caller0.Update(ip2, listener3)
	net0.Track(ip2.IP())

	// Confirm that id2 was added as peer
	wg3.Wait()
	assertEqualPeers(t, map[string]ids.ShortID{
		ip2.String(): id2,
	}, net0.Peers([]ids.ShortID{}))
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net1.Peers([]ids.ShortID{}))
	assert.Len(t, net2.Peers([]ids.ShortID{}), 0)
	assertEqualPeers(t, map[string]ids.ShortID{
		ip0.String(): id0,
	}, net3.Peers([]ids.ShortID{}))

	// Cleanup
	cleanup = true
	err := net0.Close()
	assert.NoError(t, err)

	err = net1.Close()
	assert.NoError(t, err)

	err = net2.Close()
	assert.NoError(t, err)

	err = net3.Close()
	assert.NoError(t, err)
}
