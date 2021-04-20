// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package node

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ava-labs/avalanchego/api/admin"
	"github.com/ava-labs/avalanchego/api/auth"
	"github.com/ava-labs/avalanchego/api/health"
	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/api/keystore"
	"github.com/ava-labs/avalanchego/api/metrics"
	"github.com/ava-labs/avalanchego/api/server"
	"github.com/ava-labs/avalanchego/chains"
	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/meterdb"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/genesis"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/indexer"
	"github.com/ava-labs/avalanchego/ipcs"
	"github.com/ava-labs/avalanchego/network"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/networking/benchlist"
	"github.com/ava-labs/avalanchego/snow/networking/router"
	"github.com/ava-labs/avalanchego/snow/networking/timeout"
	"github.com/ava-labs/avalanchego/snow/triggers"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/math"
	"github.com/ava-labs/avalanchego/utils/timer"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/version"
	"github.com/ava-labs/avalanchego/vms"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/avalanchego/vms/evm"
	"github.com/ava-labs/avalanchego/vms/nftfx"
	"github.com/ava-labs/avalanchego/vms/platformvm"
	"github.com/ava-labs/avalanchego/vms/propertyfx"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
	"github.com/ava-labs/avalanchego/vms/timestampvm"

	ipcsapi "github.com/ava-labs/avalanchego/api/ipcs"
)

// Networking constants
const (
	TCP = "tcp"
)

var (
	genesisHashKey                  = []byte("genesisID")
	indexerDBPrefix                 = []byte{0x00}
	errPrimarySubnetNotBootstrapped = errors.New("primary subnet has not finished bootstrapping")

	beaconConnectionTimeout = 1 * time.Minute
)

// Node is an instance of an Avalanche node.
type Node struct {
	Log        logging.Logger
	LogFactory logging.Factory
	HTTPLog    logging.Logger

	// This node's unique ID used when communicating with other nodes
	// (in consensus, for example)
	ID ids.ShortID

	// Storage for this node
	DB database.Database

	// Indexes blocks, transactions and blocks
	indexer indexer.Indexer

	// Handles calls to Keystore API
	keystore keystore.Keystore

	// Manages shared memory
	sharedMemory atomic.Memory

	// Monitors node health and runs health checks
	healthService health.Service

	// Manages creation of blockchains and routing messages to them
	chainManager chains.Manager

	// Manages Virtual Machines
	vmManager vms.Manager

	// Manages validator benching
	benchlistManager benchlist.Manager

	// dispatcher for events as they happen in consensus
	DecisionDispatcher  *triggers.EventDispatcher
	ConsensusDispatcher *triggers.EventDispatcher

	IPCs *ipcs.ChainIPCs

	// Net runs the networking stack
	Net network.Network

	// this node's initial connections to the network
	beacons validators.Set

	// current validators of the network
	vdrs validators.Manager

	// Handles HTTP API calls
	APIServer server.Server

	// This node's configuration
	Config *Config

	// channel for closing the node
	nodeCloser chan<- os.Signal

	// ensures that we only close the node once.
	shutdownOnce sync.Once

	// True if node is shutting down or is done shutting down
	shuttingDown utils.AtomicBool

	// Incremented only once on initialization.
	// Decremented when node is done shutting down.
	doneShuttingDown sync.WaitGroup

	// Restarter can shutdown and restart the node
	restarter utils.Restarter
}

/*
 ******************************************************************************
 *************************** P2P Networking Section ***************************
 ******************************************************************************
 */

func (n *Node) initNetworking() error {
	listener, err := net.Listen(TCP, fmt.Sprintf(":%d", n.Config.StakingIP.Port))
	if err != nil {
		return err
	}
	dialer := network.NewDialer(TCP)

	var serverUpgrader, clientUpgrader network.Upgrader
	if n.Config.EnableP2PTLS {
		cert, err := tls.LoadX509KeyPair(n.Config.StakingCertFile, n.Config.StakingKeyFile)
		if err != nil {
			return err
		}

		// #nosec G402
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAnyClientCert,
			// We do not use the TLS CA functionality to authenticate a
			// hostname. We only require an authenticated channel based on the
			// peer's public key. Therefore, we can safely skip CA verification.
			//
			// During our security audit by Quantstamp, this was investigated
			// and confirmed to be safe and correct.
			InsecureSkipVerify: true,
		}

		serverUpgrader = network.NewTLSServerUpgrader(tlsConfig)
		clientUpgrader = network.NewTLSClientUpgrader(tlsConfig)
	} else {
		serverUpgrader = network.NewIPUpgrader()
		clientUpgrader = network.NewIPUpgrader()
	}

	// Initialize validator manager and primary network's validator set
	primaryNetworkValidators := validators.NewSet()
	n.vdrs = validators.NewManager()
	if err := n.vdrs.Set(constants.PrimaryNetworkID, primaryNetworkValidators); err != nil {
		return err
	}

	// Configure benchlist
	n.Config.BenchlistConfig.Validators = n.vdrs
	n.benchlistManager = benchlist.NewManager(&n.Config.BenchlistConfig)

	consensusRouter := n.Config.ConsensusRouter
	if !n.Config.EnableStaking {
		if err := primaryNetworkValidators.AddWeight(n.ID, n.Config.DisabledStakingWeight); err != nil {
			return err
		}
		consensusRouter = &insecureValidatorManager{
			Router: consensusRouter,
			vdrs:   primaryNetworkValidators,
			weight: n.Config.DisabledStakingWeight,
		}
	}

	bootstrapWeight := n.beacons.Weight()
	reqWeight := (3*bootstrapWeight + 3) / 4

	if reqWeight > 0 {
		// Set a timer that will fire after a given timeout unless we connect
		// to a sufficient portion of stake-weighted nodes. If the timeout
		// fires, the node will shutdown.
		timer := timer.NewTimer(func() {
			// If the timeout fires and we're already shutting down, nothing to do.
			if !n.shuttingDown.GetValue() {
				n.Log.Warn("Failed to connect to bootstrap nodes. Node shutting down...")
				go n.Shutdown()
			}
		})

		go timer.Dispatch()
		timer.SetTimeoutIn(beaconConnectionTimeout)

		consensusRouter = &beaconManager{
			Router:         consensusRouter,
			timer:          timer,
			beacons:        n.beacons,
			requiredWeight: reqWeight,
		}
	}

	versionManager := version.NewCompatibility(
		Version,
		MinimumCompatibleVersion,
		GetApricotPhase1Time(n.Config.NetworkID),
		PrevMinimumCompatibleVersion,
		MinimumUnmaskedVersion,
		GetApricotPhase0Time(n.Config.NetworkID),
		PrevMinimumUnmaskedVersion,
	)

	n.Net = network.NewDefaultNetwork(
		n.Config.ConsensusParams.Metrics,
		n.Log,
		n.ID,
		n.Config.StakingIP,
		n.Config.NetworkID,
		versionManager,
		VersionParser,
		listener,
		dialer,
		serverUpgrader,
		clientUpgrader,
		primaryNetworkValidators,
		n.beacons,
		consensusRouter,
		n.Config.ConnMeterResetDuration,
		n.Config.ConnMeterMaxConns,
		n.restarter,
		n.Config.RestartOnDisconnected,
		n.Config.DisconnectedCheckFreq,
		n.Config.DisconnectedRestartTimeout,
		n.Config.SendQueueSize,
		n.Config.NetworkHealthConfig,
		n.benchlistManager,
		n.Config.PeerAliasTimeout,
	)

	n.nodeCloser = utils.HandleSignals(func(os.Signal) {
		// errors are already logged internally if they are meaningful
		n.Shutdown()
	}, syscall.SIGINT, syscall.SIGTERM)

	return nil
}

type insecureValidatorManager struct {
	router.Router
	vdrs   validators.Set
	weight uint64
}

func (i *insecureValidatorManager) Connected(vdrID ids.ShortID) {
	_ = i.vdrs.AddWeight(vdrID, i.weight)
	i.Router.Connected(vdrID)
}

func (i *insecureValidatorManager) Disconnected(vdrID ids.ShortID) {
	// Shouldn't error unless the set previously had an error, which should
	// never happen as described above
	_ = i.vdrs.RemoveWeight(vdrID, i.weight)
	i.Router.Disconnected(vdrID)
}

type beaconManager struct {
	router.Router
	timer          *timer.Timer
	beacons        validators.Set
	requiredWeight uint64
	weight         uint64
}

func (b *beaconManager) Connected(vdrID ids.ShortID) {
	weight, ok := b.beacons.GetWeight(vdrID)
	if !ok {
		b.Router.Connected(vdrID)
		return
	}
	weight, err := math.Add64(weight, b.weight)
	if err != nil {
		b.timer.Cancel()
		b.Router.Connected(vdrID)
		return
	}
	b.weight = weight
	if b.weight >= b.requiredWeight {
		b.timer.Cancel()
	}
	b.Router.Connected(vdrID)
}

func (b *beaconManager) Disconnected(vdrID ids.ShortID) {
	if weight, ok := b.beacons.GetWeight(vdrID); ok {
		// TODO: Account for weight changes in a more robust manner.

		// Sub64 should rarely error since only validators that have added their
		// weight can become disconnected. Because it is possible that there are
		// changes to the validators set, we utilize that Sub64 returns 0 on
		// error.
		b.weight, _ = math.Sub64(b.weight, weight)
	}
	b.Router.Disconnected(vdrID)
}

// Dispatch starts the node's servers.
// Returns when the node exits.
func (n *Node) Dispatch() error {
	// Start the HTTP API server
	go n.Log.RecoverAndPanic(func() {
		var err error
		if n.Config.HTTPSEnabled {
			n.Log.Debug("initializing API server with TLS")
			err = n.APIServer.DispatchTLS(n.Config.HTTPSCertFile, n.Config.HTTPSKeyFile)
		} else {
			n.Log.Debug("initializing API server without TLS")
			err = n.APIServer.Dispatch()
		}
		// When [n].Shutdown() is called, [n.APIServer].Close() is called.
		// This causes [n.APIServer].Dispatch() to return an error.
		// If that happened, don't log/return an error here.
		if !n.shuttingDown.GetValue() {
			n.Log.Fatal("API server dispatch failed with %s", err)
		}
		// If the API server isn't running, shut down the node.
		// If node is already shutting down, this does nothing.
		n.Shutdown()
	})

	// Add bootstrap nodes to the peer network
	for _, peer := range n.Config.BootstrapPeers {
		if !peer.IP.Equal(n.Config.StakingIP.IP()) {
			n.Net.Track(peer.IP)
		} else {
			n.Log.Error("can't add self as a bootstrapper")
		}
	}

	// Start P2P connections
	err := n.Net.Dispatch()

	// If the P2P server isn't running, shut down the node.
	// If node is already shutting down, this does nothing.
	n.Shutdown()

	// Wait until the node is done shutting down before returning
	n.doneShuttingDown.Wait()
	return err
}

/*
 ******************************************************************************
 *********************** End P2P Networking Section ***************************
 ******************************************************************************
 */

func (n *Node) initDatabase(db database.Database) error {
	n.DB = db

	rawExpectedGenesisHash := hashing.ComputeHash256(n.Config.GenesisBytes)

	rawGenesisHash, err := n.DB.Get(genesisHashKey)
	if err == database.ErrNotFound {
		rawGenesisHash = rawExpectedGenesisHash
		err = n.DB.Put(genesisHashKey, rawGenesisHash)
	}
	if err != nil {
		return err
	}

	genesisHash, err := ids.ToID(rawGenesisHash)
	if err != nil {
		return err
	}
	expectedGenesisHash, err := ids.ToID(rawExpectedGenesisHash)
	if err != nil {
		return err
	}

	if genesisHash != expectedGenesisHash {
		return fmt.Errorf("db contains invalid genesis hash. DB Genesis: %s Generated Genesis: %s", genesisHash, expectedGenesisHash)
	}
	return nil
}

// Initialize this node's ID
// If staking is disabled, a node's ID is a hash of its IP
// Otherwise, it is a hash of the TLS certificate that this node
// uses for P2P communication
func (n *Node) initNodeID() error {
	if !n.Config.EnableP2PTLS {
		n.ID = ids.ShortID(hashing.ComputeHash160Array([]byte(n.Config.StakingIP.IP().String())))
		n.Log.Info("Set the node's ID to %s", n.ID.PrefixedString(constants.NodeIDPrefix))
		return nil
	}

	stakeCert, err := ioutil.ReadFile(n.Config.StakingCertFile)
	if err != nil {
		return fmt.Errorf("problem reading staking certificate: %w", err)
	}

	block, _ := pem.Decode(stakeCert)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("problem parsing staking certificate: %w", err)
	}
	n.ID, err = ids.ToShortID(hashing.PubkeyBytesToAddress(cert.Raw))
	if err != nil {
		return fmt.Errorf("problem deriving staker ID from certificate: %w", err)
	}
	n.Log.Info("Set node's ID to %s", n.ID.PrefixedString(constants.NodeIDPrefix))
	return nil
}

// Set the node IDs of the peers this node should first connect to
func (n *Node) initBeacons() error {
	n.beacons = validators.NewSet()
	for _, peer := range n.Config.BootstrapPeers {
		if err := n.beacons.AddWeight(peer.ID, 1); err != nil {
			return err
		}
	}
	return nil
}

// Create the EventDispatcher used for hooking events
// into the general process flow.
func (n *Node) initEventDispatcher() error {
	n.DecisionDispatcher = &triggers.EventDispatcher{}
	n.DecisionDispatcher.Initialize(n.Log)

	n.ConsensusDispatcher = &triggers.EventDispatcher{}
	n.ConsensusDispatcher.Initialize(n.Log)

	return n.ConsensusDispatcher.Register("gossip", n.Net)
}

func (n *Node) initIPCs() error {
	chainIDs := make([]ids.ID, len(n.Config.IPCDefaultChainIDs))
	for i, chainID := range n.Config.IPCDefaultChainIDs {
		id, err := ids.FromString(chainID)
		if err != nil {
			return err
		}
		chainIDs[i] = id
	}

	var err error
	n.IPCs, err = ipcs.NewChainIPCs(n.Log, n.Config.IPCPath, n.Config.NetworkID, n.ConsensusDispatcher, n.DecisionDispatcher, chainIDs)
	return err
}

// Initialize [n.indexer].
// Should only be called after [n.DB], [n.DecisionDispatcher], [n.ConsensusDispatcher],
// [n.Log], [n.APIServer], [n.chainManager] are initialized
func (n *Node) initIndexer() error {
	txIndexerDB := prefixdb.New(indexerDBPrefix, n.DB)
	var err error
	n.indexer, err = indexer.NewIndexer(indexer.Config{
		IndexingEnabled:      n.Config.IndexAPIEnabled,
		AllowIncompleteIndex: n.Config.IndexAllowIncomplete,
		DB:                   txIndexerDB,
		Log:                  n.Log,
		DecisionDispatcher:   n.DecisionDispatcher,
		ConsensusDispatcher:  n.ConsensusDispatcher,
		APIServer:            &n.APIServer,
		ShutdownF:            n.Shutdown,
	})
	if err != nil {
		return fmt.Errorf("couldn't create index for txs: %w", err)
	}

	// Chain manager will notify indexer when a chain is created
	n.chainManager.AddRegistrant(n.indexer)

	return nil
}

// Initializes the Platform chain.
// Its genesis data specifies the other chains that should be created.
func (n *Node) initChains(genesisBytes []byte) {
	n.Log.Info("initializing chains")

	// Create the Platform Chain
	n.chainManager.ForceCreateChain(chains.ChainParameters{
		ID:            constants.PlatformChainID,
		SubnetID:      constants.PrimaryNetworkID,
		GenesisData:   genesisBytes, // Specifies other chains to create
		VMAlias:       platformvm.ID.String(),
		CustomBeacons: n.beacons,
	})
}

// initAPIServer initializes the server that handles HTTP calls
func (n *Node) initAPIServer() error {
	n.Log.Info("initializing API server")

	if !n.Config.APIRequireAuthToken {
		n.APIServer.Initialize(
			n.Log,
			n.LogFactory,
			n.Config.HTTPHost,
			n.Config.HTTPPort,
			n.Config.APIAllowedOrigins,
		)
		return nil
	}

	a, err := auth.New(n.Log, "auth", n.Config.APIAuthPassword)
	if err != nil {
		return err
	}

	n.APIServer.Initialize(
		n.Log,
		n.LogFactory,
		n.Config.HTTPHost,
		n.Config.HTTPPort,
		n.Config.APIAllowedOrigins,
		a,
	)

	// only create auth service if token authorization is required
	n.Log.Info("API authorization is enabled. Auth tokens must be passed in the header of API requests, except requests to the auth service.")
	authService, err := a.CreateHandler()
	if err != nil {
		return err
	}
	handler := &common.HTTPHandler{
		LockOptions: common.NoLock,
		Handler:     authService,
	}
	return n.APIServer.AddRoute(handler, &sync.RWMutex{}, "auth", "", n.Log)
}

// Create the vmManager, chainManager and register the following VMs:
// AVM, Simple Payments DAG, Simple Payments Chain, and Platform VM
// Assumes n.DB, n.vdrs all initialized (non-nil)
func (n *Node) initChainManager(avaxAssetID ids.ID) error {
	n.vmManager = vms.NewManager(&n.APIServer, n.HTTPLog)

	createAVMTx, err := genesis.VMGenesis(n.Config.GenesisBytes, avm.ID)
	if err != nil {
		return err
	}
	xChainID := createAVMTx.ID()

	createEVMTx, err := genesis.VMGenesis(n.Config.GenesisBytes, evm.ID)
	if err != nil {
		return err
	}
	cChainID := createEVMTx.ID()

	// If any of these chains die, the node shuts down
	criticalChains := ids.Set{}
	criticalChains.Add(
		constants.PlatformChainID,
		xChainID,
		cChainID,
	)

	// Manages network timeouts
	timeoutManager := &timeout.Manager{}
	if err := timeoutManager.Initialize(&n.Config.NetworkConfig, n.benchlistManager); err != nil {
		return err
	}
	go n.Log.RecoverAndPanic(timeoutManager.Dispatch)

	// Routes incoming messages from peers to the appropriate chain
	err = n.Config.ConsensusRouter.Initialize(
		n.ID,
		n.Log,
		timeoutManager,
		n.Config.ConsensusGossipFrequency,
		n.Config.ConsensusShutdownTimeout,
		criticalChains,
		n.Shutdown,
		n.Config.RouterHealthConfig,
		n.Config.NetworkConfig.MetricsNamespace,
		n.Config.NetworkConfig.Registerer,
	)
	if err != nil {
		return fmt.Errorf("couldn't initialize chain router: %w", err)
	}

	n.chainManager = chains.New(&chains.ManagerConfig{
		StakingEnabled:            n.Config.EnableStaking,
		MaxPendingMsgs:            n.Config.MaxPendingMsgs,
		MaxNonStakerPendingMsgs:   n.Config.MaxNonStakerPendingMsgs,
		StakerMSGPortion:          n.Config.StakerMSGPortion,
		StakerCPUPortion:          n.Config.StakerCPUPortion,
		Log:                       n.Log,
		LogFactory:                n.LogFactory,
		VMManager:                 n.vmManager,
		DecisionEvents:            n.DecisionDispatcher,
		ConsensusEvents:           n.ConsensusDispatcher,
		DB:                        n.DB,
		Router:                    n.Config.ConsensusRouter,
		Net:                       n.Net,
		ConsensusParams:           n.Config.ConsensusParams,
		EpochFirstTransition:      n.Config.EpochFirstTransition,
		EpochDuration:             n.Config.EpochDuration,
		Validators:                n.vdrs,
		NodeID:                    n.ID,
		NetworkID:                 n.Config.NetworkID,
		Server:                    &n.APIServer,
		Keystore:                  n.keystore,
		AtomicMemory:              &n.sharedMemory,
		AVAXAssetID:               avaxAssetID,
		XChainID:                  xChainID,
		CriticalChains:            criticalChains,
		TimeoutManager:            timeoutManager,
		HealthService:             n.healthService,
		WhitelistedSubnets:        n.Config.WhitelistedSubnets,
		RetryBootstrap:            n.Config.RetryBootstrap,
		RetryBootstrapMaxAttempts: n.Config.RetryBootstrapMaxAttempts,
	})

	vdrs := n.vdrs

	// If staking is disabled, ignore updates to Subnets' validator sets
	// Instead of updating node's validator manager, platform chain makes changes
	// to its own local validator manager (which isn't used for sampling)
	if !n.Config.EnableStaking {
		vdrs = validators.NewManager()
	}

	// Register the VMs that Avalanche supports
	errs := wrappers.Errs{}
	errs.Add(
		n.vmManager.RegisterVMFactory(platformvm.ID, &platformvm.Factory{
			ChainManager:       n.chainManager,
			Validators:         vdrs,
			StakingEnabled:     n.Config.EnableStaking,
			CreationFee:        n.Config.CreationTxFee,
			Fee:                n.Config.TxFee,
			UptimePercentage:   n.Config.UptimeRequirement,
			MinValidatorStake:  n.Config.MinValidatorStake,
			MaxValidatorStake:  n.Config.MaxValidatorStake,
			MinDelegatorStake:  n.Config.MinDelegatorStake,
			MinDelegationFee:   n.Config.MinDelegationFee,
			MinStakeDuration:   n.Config.MinStakeDuration,
			MaxStakeDuration:   n.Config.MaxStakeDuration,
			StakeMintingPeriod: n.Config.StakeMintingPeriod,
			ApricotPhase0Time:  GetApricotPhase0Time(n.Config.NetworkID),
		}),
		n.vmManager.RegisterVMFactory(avm.ID, &avm.Factory{
			CreationFee: n.Config.CreationTxFee,
			Fee:         n.Config.TxFee,
		}),
		n.vmManager.RegisterVMFactory(evm.ID, &rpcchainvm.Factory{
			Path:   filepath.Join(n.Config.PluginDir, "evm"),
			Config: n.Config.CorethConfig,
		}),
		n.vmManager.RegisterVMFactory(timestampvm.ID, &timestampvm.Factory{}),
		n.vmManager.RegisterVMFactory(secp256k1fx.ID, &secp256k1fx.Factory{}),
		n.vmManager.RegisterVMFactory(nftfx.ID, &nftfx.Factory{}),
		n.vmManager.RegisterVMFactory(propertyfx.ID, &propertyfx.Factory{}),
	)
	if errs.Errored() {
		return errs.Err
	}

	// Notify the API server when new chains are created
	n.chainManager.AddRegistrant(&n.APIServer)
	return nil
}

// initSharedMemory initializes the shared memory for cross chain interation
func (n *Node) initSharedMemory() error {
	n.Log.Info("initializing SharedMemory")
	sharedMemoryDB := prefixdb.New([]byte("shared memory"), n.DB)
	return n.sharedMemory.Initialize(n.Log, sharedMemoryDB)
}

// initKeystoreAPI initializes the keystore service, which is an on-node wallet.
// Assumes n.APIServer is already set
func (n *Node) initKeystoreAPI() error {
	n.Log.Info("initializing keystore")
	keystoreDB := prefixdb.New([]byte("keystore"), n.DB)
	n.keystore = keystore.New(n.Log, keystoreDB)
	keystoreHandler, err := n.keystore.CreateHandler()
	if err != nil {
		return err
	}
	if !n.Config.KeystoreAPIEnabled {
		n.Log.Info("skipping keystore API initialization because it has been disabled")
		return nil
	}
	n.Log.Info("initializing keystore API")
	handler := &common.HTTPHandler{
		LockOptions: common.NoLock,
		Handler:     keystoreHandler,
	}
	return n.APIServer.AddRoute(handler, &sync.RWMutex{}, "keystore", "", n.HTTPLog)
}

// initMetricsAPI initializes the Metrics API
// Assumes n.APIServer is already set
func (n *Node) initMetricsAPI() error {
	registry, handler := metrics.NewService()
	// It is assumed by components of the system that the Metrics interface is
	// non-nil. So, it is set regardless of if the metrics API is available or not.
	n.Config.ConsensusParams.Metrics = registry
	n.Config.NetworkConfig.MetricsNamespace = constants.PlatformName
	n.Config.NetworkConfig.Registerer = registry

	if !n.Config.MetricsAPIEnabled {
		n.Log.Info("skipping metrics API initialization because it has been disabled")
		return nil
	}

	n.Log.Info("initializing metrics API")

	dbNamespace := fmt.Sprintf("%s_db", constants.PlatformName)
	db, err := meterdb.New(dbNamespace, registry, n.DB)
	if err != nil {
		return err
	}
	n.DB = db

	return n.APIServer.AddRoute(handler, &sync.RWMutex{}, "metrics", "", n.HTTPLog)
}

// initAdminAPI initializes the Admin API service
// Assumes n.log, n.chainManager, and n.ValidatorAPI already initialized
func (n *Node) initAdminAPI() error {
	if !n.Config.AdminAPIEnabled {
		n.Log.Info("skipping admin API initialization because it has been disabled")
		return nil
	}
	n.Log.Info("initializing admin API")
	service, err := admin.NewService(n.Log, n.chainManager, &n.APIServer)
	if err != nil {
		return err
	}
	return n.APIServer.AddRoute(service, &sync.RWMutex{}, "admin", "", n.HTTPLog)
}

func (n *Node) initInfoAPI() error {
	if !n.Config.InfoAPIEnabled {
		n.Log.Info("skipping info API initialization because it has been disabled")
		return nil
	}
	n.Log.Info("initializing info API")
	service, err := info.NewService(
		n.Log,
		Version,
		n.ID,
		n.Config.NetworkID,
		n.chainManager,
		n.Net,
		n.Config.CreationTxFee,
		n.Config.TxFee,
	)
	if err != nil {
		return err
	}
	return n.APIServer.AddRoute(service, &sync.RWMutex{}, "info", "", n.HTTPLog)
}

// initHealthAPI initializes the Health API service
// Assumes n.Log, n.Net, n.APIServer, n.HTTPLog already initialized
func (n *Node) initHealthAPI() error {
	if !n.Config.HealthAPIEnabled {
		n.healthService = health.NewNoOpService()
		n.Log.Info("skipping health API initialization because it has been disabled")
		return nil
	}

	n.Log.Info("initializing Health API")
	healthService, err := health.NewService(n.Config.HealthCheckFreq, n.Log, n.Config.NetworkConfig.MetricsNamespace, n.Config.ConsensusParams.Metrics)
	if err != nil {
		return err
	}
	n.healthService = healthService

	isBootstrappedFunc := func() (interface{}, error) {
		if pChainID, err := n.chainManager.Lookup("P"); err != nil {
			return nil, errors.New("P-Chain not created")
		} else if xChainID, err := n.chainManager.Lookup("X"); err != nil {
			return nil, errors.New("X-Chain not created")
		} else if cChainID, err := n.chainManager.Lookup("C"); err != nil {
			return nil, errors.New("C-Chain not created")
		} else if !n.chainManager.IsBootstrapped(pChainID) || !n.chainManager.IsBootstrapped(xChainID) || !n.chainManager.IsBootstrapped(cChainID) {
			return nil, errPrimarySubnetNotBootstrapped
		}

		return nil, nil
	}
	// Passes if the P, X and C chains are finished bootstrapping
	err = n.healthService.RegisterMonotonicCheck("isBootstrapped", isBootstrappedFunc)
	if err != nil {
		return fmt.Errorf("couldn't register isBootstrapped health check: %w", err)
	}

	// Register the network layer with the health service
	err = n.healthService.RegisterCheck("network", n.Net.HealthCheck)
	if err != nil {
		return fmt.Errorf("couldn't register network health check")
	}

	// Register the router with the health service
	err = n.healthService.RegisterCheck("router", n.Config.ConsensusRouter.HealthCheck)
	if err != nil {
		return fmt.Errorf("couldn't register router health check")
	}

	handler, err := n.healthService.Handler()
	if err != nil {
		return err
	}

	return n.APIServer.AddRoute(handler, &sync.RWMutex{}, "health", "", n.HTTPLog)
}

// initIPCAPI initializes the IPC API service
// Assumes n.log and n.chainManager already initialized
func (n *Node) initIPCAPI() error {
	if !n.Config.IPCAPIEnabled {
		n.Log.Info("skipping ipc API initialization because it has been disabled")
		return nil
	}
	n.Log.Info("initializing ipc API")
	service, err := ipcsapi.NewService(n.Log, n.chainManager, &n.APIServer, n.IPCs)
	if err != nil {
		return err
	}
	return n.APIServer.AddRoute(service, &sync.RWMutex{}, "ipcs", "", n.HTTPLog)
}

// Give chains and VMs aliases as specified by the genesis information
func (n *Node) initAliases(genesisBytes []byte) error {
	n.Log.Info("initializing aliases")
	defaultAliases, chainAliases, vmAliases, err := genesis.Aliases(genesisBytes)
	if err != nil {
		return err
	}

	for chainID, aliases := range chainAliases {
		for _, alias := range aliases {
			if err := n.chainManager.Alias(chainID, alias); err != nil {
				return err
			}
		}
	}
	for vmID, aliases := range vmAliases {
		for _, alias := range aliases {
			if err := n.vmManager.Alias(vmID, alias); err != nil {
				return err
			}
		}
	}
	for url, aliases := range defaultAliases {
		if err := n.APIServer.AddAliases(url, aliases...); err != nil {
			return err
		}
	}
	return nil
}

// Initialize this node
func (n *Node) Initialize(
	config *Config,
	db database.Database,
	logger logging.Logger,
	logFactory logging.Factory,
	restarter utils.Restarter,
) error {
	n.Log = logger
	n.LogFactory = logFactory
	n.Config = config
	n.restarter = restarter
	n.doneShuttingDown.Add(1)
	n.Log.Info("Node version is: %s", Version)

	httpLog, err := logFactory.MakeSubdir("http")
	if err != nil {
		return fmt.Errorf("problem initializing HTTP logger: %w", err)
	}
	n.HTTPLog = httpLog

	if err := n.initDatabase(db); err != nil { // Set up the node's database
		return fmt.Errorf("problem initializing database: %w", err)
	}
	if err = n.initNodeID(); err != nil { // Derive this node's ID
		return fmt.Errorf("problem initializing staker ID: %w", err)
	}
	if err = n.initBeacons(); err != nil { // Configure the beacons
		return fmt.Errorf("problem initializing node beacons: %w", err)
	}
	// Start HTTP APIs
	if err := n.initAPIServer(); err != nil { // Start the API Server
		return fmt.Errorf("couldn't initialize API server: %w", err)
	}
	if err := n.initKeystoreAPI(); err != nil { // Start the Keystore API
		return fmt.Errorf("couldn't initialize keystore API: %w", err)
	}
	if err := n.initMetricsAPI(); err != nil { // Start the Metrics API
		return fmt.Errorf("couldn't initialize metrics API: %w", err)
	}

	if err := n.initSharedMemory(); err != nil { // Initialize shared memory
		return fmt.Errorf("problem initializing shared memory: %w", err)
	}

	if err = n.initNetworking(); err != nil { // Set up all networking
		return fmt.Errorf("problem initializing networking: %w", err)
	}
	if err = n.initEventDispatcher(); err != nil { // Set up the event dipatcher
		return fmt.Errorf("problem initializing event dispatcher: %w", err)
	}

	// Start the Health API
	// Has to be initialized before chain manager
	// [n.Net] must already be set
	if err := n.initHealthAPI(); err != nil {
		return fmt.Errorf("couldn't initialize health API: %w", err)
	}
	if err := n.initChainManager(n.Config.AvaxAssetID); err != nil { // Set up the chain manager
		return fmt.Errorf("couldn't initialize chain manager: %w", err)
	}
	if err := n.initAdminAPI(); err != nil { // Start the Admin API
		return fmt.Errorf("couldn't initialize admin API: %w", err)
	}
	if err := n.initInfoAPI(); err != nil { // Start the Info API
		return fmt.Errorf("couldn't initialize info API: %w", err)
	}
	if err := n.initIPCs(); err != nil { // Start the IPCs
		return fmt.Errorf("couldn't initialize IPCs: %w", err)
	}
	if err := n.initIPCAPI(); err != nil { // Start the IPC API
		return fmt.Errorf("couldn't initialize the IPC API: %w", err)
	}
	if err := n.initAliases(n.Config.GenesisBytes); err != nil { // Set up aliases
		return fmt.Errorf("couldn't initialize aliases: %w", err)
	}
	if err := n.initIndexer(); err != nil {
		return fmt.Errorf("couldn't initialize indexer: %w", err)
	}
	// Start the Platform chain
	n.initChains(n.Config.GenesisBytes)
	return nil
}

// Shutdown this node
// May be called multiple times
func (n *Node) Shutdown() {
	n.shuttingDown.SetValue(true)
	n.shutdownOnce.Do(n.shutdown)
}

func (n *Node) shutdown() {
	n.Log.Info("shutting down node")
	if n.IPCs != nil {
		if err := n.IPCs.Shutdown(); err != nil {
			n.Log.Debug("error during IPC shutdown: %s", err)
		}
	}
	if n.chainManager != nil {
		n.chainManager.Shutdown()
	}
	if n.Net != nil {
		// Close already logs its own error if one occurs, so the error is ignored here
		_ = n.Net.Close()
	}
	if err := n.APIServer.Shutdown(); err != nil {
		n.Log.Debug("error during API shutdown: %s", err)
	}
	if err := n.indexer.Close(); err != nil {
		n.Log.Debug("error closing tx indexer: %w", err)
	}
	utils.ClearSignals(n.nodeCloser)
	n.doneShuttingDown.Done()
	n.Log.Info("finished node shutdown")
}
