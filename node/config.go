// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package node

import (
	"time"

	"github.com/ava-labs/avalanchego/genesis"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/nat"
	"github.com/ava-labs/avalanchego/network"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/networking/benchlist"
	"github.com/ava-labs/avalanchego/snow/networking/router"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/dynamicip"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/timer"
)

// Config contains all of the configurations of an Avalanche node.
type Config struct {
	genesis.Params

	// Genesis information
	GenesisBytes []byte
	AvaxAssetID  ids.ID

	// protocol to use for opening the network interface
	Nat nat.Router

	// Attempted NAT Traversal did we attempt
	AttemptedNATTraversal bool

	// ID of the network this node should connect to
	NetworkID uint32

	// Assertions configuration
	EnableAssertions bool

	// Crypto configuration
	EnableCrypto bool

	// Path to database
	DBPath string

	// If false, uses an in memory database
	DBEnabled bool

	// Staking configuration
	StakingIP             utils.DynamicIPDesc
	EnableP2PTLS          bool
	EnableStaking         bool
	StakingKeyFile        string
	StakingCertFile       string
	DisabledStakingWeight uint64

	// Throttling
	MaxNonStakerPendingMsgs uint32
	StakerMSGPortion        float64
	StakerCPUPortion        float64
	SendQueueSize           uint32
	MaxPendingMsgs          uint32

	// Health
	HealthCheckFreq time.Duration

	// Network configuration
	NetworkConfig       timer.AdaptiveTimeoutConfig
	NetworkHealthConfig network.HealthConfig

	// Benchlist Configuration
	BenchlistConfig benchlist.Config

	// Bootstrapping configuration
	BootstrapPeers []*Peer

	// HTTP configuration
	HTTPHost string
	HTTPPort uint16

	HTTPSEnabled        bool
	HTTPSKeyFile        string
	HTTPSCertFile       string
	APIRequireAuthToken bool
	APIAuthPassword     string
	APIAllowedOrigins   []string

	// Enable/Disable APIs
	AdminAPIEnabled    bool
	InfoAPIEnabled     bool
	KeystoreAPIEnabled bool
	MetricsAPIEnabled  bool
	HealthAPIEnabled   bool
	IndexAPIEnabled    bool

	// Logging configuration
	LoggingConfig logging.Config

	// Plugin directory
	PluginDir string

	// Consensus configuration
	ConsensusParams avalanche.Parameters

	// Throughput configuration
	ThroughputPort          uint16
	ThroughputServerEnabled bool

	// IPC configuration
	IPCAPIEnabled      bool
	IPCPath            string
	IPCDefaultChainIDs []string

	// Router that is used to handle incoming consensus messages
	ConsensusRouter          router.Router
	RouterHealthConfig       router.HealthConfig
	ConsensusGossipFrequency time.Duration
	ConsensusShutdownTimeout time.Duration

	// Dynamic Update duration for IP or NAT traversal
	DynamicUpdateDuration time.Duration

	DynamicPublicIPResolver dynamicip.Resolver

	// Throttling incoming connections
	ConnMeterResetDuration time.Duration
	ConnMeterMaxConns      int

	// Subnet Whitelist
	WhitelistedSubnets ids.Set

	// Restart on disconnect settings
	RestartOnDisconnected      bool
	DisconnectedCheckFreq      time.Duration
	DisconnectedRestartTimeout time.Duration

	// Coreth
	CorethConfig string

	IndexAllowIncomplete bool

	// Should Bootstrap be retried
	RetryBootstrap bool

	// Max number of times to retry bootstrap
	RetryBootstrapMaxAttempts int

	// Peer alias configuration
	PeerAliasTimeout time.Duration
}
