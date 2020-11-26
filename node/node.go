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

	"github.com/ava-labs/avalanchego/vms/avm"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/api/admin"
	"github.com/ava-labs/avalanchego/api/health"
	"github.com/ava-labs/avalanchego/api/info"
	"github.com/ava-labs/avalanchego/api/keystore"
	"github.com/ava-labs/avalanchego/api/metrics"
	"github.com/ava-labs/avalanchego/chains"
	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/meterdb"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/genesis"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/ipcs"
	"github.com/ava-labs/avalanchego/network"
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
	genesisHashKey = []byte("genesisID")

	// Version is the version of this code
	Version       = version.NewDefaultVersion(constants.PlatformName, 1, 0, 5)
	versionParser = version.NewDefaultParser()
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

	// Handles calls to Keystore API
	keystoreServer keystore.Keystore

	// Manages shared memory
	sharedMemory atomic.Memory

	// Monitors node health and runs health checks
	healthService *health.Health

	// Manages creation of blockchains and routing messages to them
	chainManager chains.Manager

	// Manages Virtual Machines
	vmManager vms.Manager

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
	APIServer api.Server

	// This node's configuration
	Config *Config

	// channel for closing the node
	nodeCloser chan<- os.Signal
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
		timer := timer.NewTimer(func() {
			n.Log.Fatal("Failed to connect to bootstrap nodes. Node shutting down...")
			go n.Net.Close()
		})

		go timer.Dispatch()
		timer.SetTimeoutIn(1 * time.Minute)

		consensusRouter = &beaconManager{
			Router:         consensusRouter,
			timer:          timer,
			beacons:        n.beacons,
			requiredWeight: reqWeight,
		}
	}

	n.Net = network.NewDefaultNetwork(
		n.Config.ConsensusParams.Metrics,
		n.Log,
		n.ID,
		n.Config.StakingIP,
		n.Config.NetworkID,
		Version,
		versionParser,
		listener,
		dialer,
		serverUpgrader,
		clientUpgrader,
		primaryNetworkValidators,
		n.beacons,
		consensusRouter,
		n.Config.ConnMeterResetDuration,
		n.Config.ConnMeterMaxConns,
	)

	n.nodeCloser = utils.HandleSignals(func(os.Signal) {
		// errors are already logged internally if they are meaningful
		_ = n.Net.Close()
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
	// Start the HTTP endpoint
	go n.Log.RecoverAndPanic(func() {
		if n.Config.HTTPSEnabled {
			n.Log.Debug("Initializing API server with TLS Enabled")
			err := n.APIServer.DispatchTLS(n.Config.HTTPSCertFile, n.Config.HTTPSKeyFile)
			n.Log.Warn("Secure API server initialization failed with %s, attempting to create insecure API server", err)
		}

		n.Log.Debug("Initializing API server")
		err := n.APIServer.Dispatch()

		n.Log.Fatal("API server initialization failed with %s", err)

		// errors are already logged internally if they are meaningful
		_ = n.Net.Close() // If the server isn't up, shut down the node.
	})

	// Add bootstrap nodes to the peer network
	for _, peer := range n.Config.BootstrapPeers {
		if !peer.IP.Equal(n.Config.StakingIP.IP()) {
			n.Net.Track(peer.IP)
		} else {
			n.Log.Error("can't add self as a bootstrapper")
		}
	}

	return n.Net.Dispatch()
}

/*
 ******************************************************************************
 *********************** End P2P Networking Section ***************************
 ******************************************************************************
 */

func (n *Node) initDatabase() error {
	n.DB = n.Config.DB

	expectedGenesis, _, err := genesis.Genesis(n.Config.NetworkID)
	if err != nil {
		return err
	}
	rawExpectedGenesisHash := hashing.ComputeHash256(expectedGenesis)

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
		n.ID = ids.NewShortID(hashing.ComputeHash160Array([]byte(n.Config.StakingIP.IP().String())))
		n.Log.Info("Set the node's ID to %s", n.ID)
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
	n.Log.Info("Set node's ID to %s", n.ID)
	return nil
}

// Create the IDs of the peers this node should first connect to
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

// Initializes the Platform chain.
// Its genesis data specifies the other chains that should
// be created.
func (n *Node) initChains(genesisBytes []byte, avaxAssetID ids.ID) error {
	n.Log.Info("initializing chains")

	// Create the Platform Chain
	n.chainManager.ForceCreateChain(chains.ChainParameters{
		ID:            constants.PlatformChainID,
		SubnetID:      constants.PrimaryNetworkID,
		GenesisData:   genesisBytes, // Specifies other chains to create
		VMAlias:       platformvm.ID.String(),
		CustomBeacons: n.beacons,
	})

	return nil
}

// initAPIServer initializes the server that handles HTTP calls
func (n *Node) initAPIServer() error {
	n.Log.Info("Initializing API server")

	return n.APIServer.Initialize(
		n.Log,
		n.LogFactory,
		n.Config.HTTPHost,
		n.Config.HTTPPort,
		n.Config.APIRequireAuthToken,
		n.Config.APIAuthPassword,
	)
}

// Create the vmManager, chainManager and register the following vms:
// AVM, Simple Payments DAG, Simple Payments Chain, and Platform VM
// Assumes n.DB, n.vdrs all initialized (non-nil)
func (n *Node) initChainManager(avaxAssetID ids.ID) error {
	n.vmManager = vms.NewManager(&n.APIServer, n.HTTPLog)

	createAVMTx, err := genesis.VMGenesis(n.Config.NetworkID, avm.ID)
	if err != nil {
		return err
	}

	xChainID := createAVMTx.ID()

	criticalChains := ids.Set{}
	criticalChains.Add(constants.PlatformChainID, createAVMTx.ID())

	n.Config.NetworkConfig.Namespace = constants.PlatformName
	n.Config.NetworkConfig.Registerer = n.Config.ConsensusParams.Metrics
	n.Config.BenchlistConfig.Validators = n.vdrs
	benchlistManager := benchlist.NewManager(&n.Config.BenchlistConfig)

	timeoutManager := timeout.Manager{}
	if err := timeoutManager.Initialize(&n.Config.NetworkConfig, benchlistManager); err != nil {
		return err
	}
	go n.Log.RecoverAndPanic(timeoutManager.Dispatch)

	n.Config.ConsensusRouter.Initialize(
		n.ID,
		n.Log,
		&timeoutManager,
		n.Config.ConsensusGossipFrequency,
		n.Config.ConsensusShutdownTimeout,
		criticalChains,
		func() {
			if err := n.Net.Close(); err != nil {
				n.Log.Debug("closing the network due to a fatal chain error resulted in: %s", err)
			}
		},
	)

	n.chainManager = chains.New(&chains.ManagerConfig{
		StakingEnabled:          n.Config.EnableStaking,
		MaxNonStakerPendingMsgs: uint32(n.Config.MaxNonStakerPendingMsgs),
		StakerMSGPortion:        n.Config.StakerMSGPortion,
		StakerCPUPortion:        n.Config.StakerCPUPortion,
		Log:                     n.Log,
		LogFactory:              n.LogFactory,
		VMManager:               n.vmManager,
		DecisionEvents:          n.DecisionDispatcher,
		ConsensusEvents:         n.ConsensusDispatcher,
		DB:                      n.DB,
		Router:                  n.Config.ConsensusRouter,
		Net:                     n.Net,
		ConsensusParams:         n.Config.ConsensusParams,
		Validators:              n.vdrs,
		NodeID:                  n.ID,
		NetworkID:               n.Config.NetworkID,
		Server:                  &n.APIServer,
		Keystore:                &n.keystoreServer,
		AtomicMemory:            &n.sharedMemory,
		AVAXAssetID:             avaxAssetID,
		XChainID:                xChainID,
		CriticalChains:          criticalChains,
		TimeoutManager:          &timeoutManager,
		HealthService:           n.healthService,
		WhitelistedSubnets:      n.Config.WhitelistedSubnets,
	})

	vdrs := n.vdrs

	// If staking is disabled, ignore updates to Subnets' validator sets
	// Instead of updating node's validator manager, platform chain makes changes
	// to its own local validator manager (which isn't used for sampling)
	if !n.Config.EnableStaking {
		vdrs = validators.NewManager()
	}

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

	n.chainManager.AddRegistrant(&n.APIServer)
	return nil
}

// initSharedMemory initializes the shared memory for cross chain interation
func (n *Node) initSharedMemory() error {
	n.Log.Info("initializing SharedMemory")
	sharedMemoryDB := prefixdb.New([]byte("shared memory"), n.DB)
	return n.sharedMemory.Initialize(n.Log, sharedMemoryDB)
}

// initKeystoreAPI initializes the keystore service
// Assumes n.APIServer is already set
func (n *Node) initKeystoreAPI() error {
	n.Log.Info("initializing keystore")
	keystoreDB := prefixdb.New([]byte("keystore"), n.DB)
	if err := n.keystoreServer.Initialize(n.Log, keystoreDB); err != nil {
		return err
	}
	keystoreHandler, err := n.keystoreServer.CreateHandler()
	if err != nil {
		return err
	}
	if !n.Config.KeystoreAPIEnabled {
		n.Log.Info("skipping keystore API initialization because it has been disabled")
		return nil
	}
	n.Log.Info("initializing keystore API")
	return n.APIServer.AddRoute(keystoreHandler, &sync.RWMutex{}, "keystore", "", n.HTTPLog)
}

// initMetricsAPI initializes the Metrics API
// Assumes n.APIServer is already set
func (n *Node) initMetricsAPI() error {
	registry, handler := metrics.NewService()
	// It is assumed by components of the system that the Metrics interface is
	// non-nil. So, it is set regardless of if the metrics API is available or not.
	n.Config.ConsensusParams.Metrics = registry
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
		n.Log.Info("skipping health API initialization because it has been disabled")
		return nil
	}
	n.Log.Info("initializing Health API")
	service := health.NewService(n.Log)
	if err := service.RegisterHeartbeat("network.validators.heartbeat", n.Net, 5*time.Minute); err != nil {
		return fmt.Errorf("couldn't register heartbeat health check: %w", err)
	}
	isBootstrappedFunc := func() (interface{}, error) {
		if pChainID, err := n.chainManager.Lookup("P"); err != nil {
			return nil, errors.New("P-Chain not created")
		} else if !n.chainManager.IsBootstrapped(pChainID) {
			return nil, errors.New("P-Chain not bootstrapped")
		}
		if xChainID, err := n.chainManager.Lookup("X"); err != nil {
			return nil, errors.New("X-Chain not created")
		} else if !n.chainManager.IsBootstrapped(xChainID) {
			return nil, errors.New("X-Chain not bootstrapped")
		}
		if cChainID, err := n.chainManager.Lookup("C"); err != nil {
			return nil, errors.New("C-Chain not created")
		} else if !n.chainManager.IsBootstrapped(cChainID) {
			return nil, errors.New("C-Chain not bootstrapped")
		}
		return nil, nil
	}
	// Passes if the P, X and C chains are finished bootstrapping
	if err := service.RegisterMonotonicCheckFunc("chains.default.bootstrapped", isBootstrappedFunc); err != nil {
		return err
	}
	handler, err := service.Handler()
	if err != nil {
		return err
	}
	n.healthService = service
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
func (n *Node) Initialize(config *Config, logger logging.Logger, logFactory logging.Factory) error {
	n.Log = logger
	n.LogFactory = logFactory
	n.Config = config
	n.Log.Info("Node version is: %s", Version)

	httpLog, err := logFactory.MakeSubdir("http")
	if err != nil {
		return fmt.Errorf("problem initializing HTTP logger: %w", err)
	}
	n.HTTPLog = httpLog

	if err := n.initDatabase(); err != nil { // Set up the node's database
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

	genesisBytes, avaxAssetID, err := genesis.Genesis(n.Config.NetworkID)
	if err != nil {
		return fmt.Errorf("couldn't create genesis bytes: %w", err)
	}
	// Start the Health API
	// Has to be initialized before chain manager
	if err := n.initHealthAPI(); err != nil {
		return fmt.Errorf("couldn't initialize health API: %w", err)
	}
	if err := n.initChainManager(avaxAssetID); err != nil { // Set up the chain manager
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
	if err := n.initAliases(genesisBytes); err != nil { // Set up aliases
		return fmt.Errorf("couldn't initialize aliases: %w", err)
	}
	if err := n.initChains(genesisBytes, avaxAssetID); err != nil { // Start the Platform chain
		return fmt.Errorf("couldn't initialize chains: %w", err)
	}
	return nil
}

// Shutdown this node
func (n *Node) Shutdown() {
	n.Log.Info("shutting down the node")
	// Close already logs its own error if one occurs, so the error is ignored
	// here
	_ = n.Net.Close()
	n.chainManager.Shutdown()
	utils.ClearSignals(n.nodeCloser)
	n.Log.Info("node shut down successfully")
}
