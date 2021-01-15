// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/ava-labs/avalanchego/database/leveldb"
	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/genesis"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/ipcs"
	"github.com/ava-labs/avalanchego/nat"
	"github.com/ava-labs/avalanchego/node"
	"github.com/ava-labs/avalanchego/snow/networking/router"
	"github.com/ava-labs/avalanchego/staking"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/dynamicip"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/password"
	"github.com/ava-labs/avalanchego/utils/ulimit"
	"github.com/ava-labs/avalanchego/utils/units"
)

const (
	dbVersion = "v1.0.0"
)

// Results of parsing the CLI
var (
	Config             = node.Config{}
	defaultNetworkName = constants.MainnetName

	homeDir                = os.ExpandEnv("$HOME")
	prefixedAppName        = fmt.Sprintf(".%s", constants.AppName)
	defaultDbDir           = filepath.Join(homeDir, prefixedAppName, "db")
	defaultStakingKeyPath  = filepath.Join(homeDir, prefixedAppName, "staking", "staker.key")
	defaultStakingCertPath = filepath.Join(homeDir, prefixedAppName, "staking", "staker.crt")
	defaultPluginDirs      = []string{
		filepath.Join(".", "build", "plugins"),
		filepath.Join(".", "plugins"),
		filepath.Join("/", "usr", "local", "lib", constants.AppName),
		filepath.Join(homeDir, prefixedAppName, "plugins"),
	}
	// GitCommit should be optionally set at compile time.
	GitCommit string
)

var (
	errBootstrapMismatch    = errors.New("more bootstrap IDs provided than bootstrap IPs")
	errStakingRequiresTLS   = errors.New("if staking is enabled, network TLS must also be enabled")
	errInvalidStakerWeights = errors.New("staking weights must be positive")
)

// avalancheFlagSet returns the complete set of flags for avalanchego
func avalancheFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(constants.AppName, flag.ContinueOnError)

	// If this is true, print the version and quit.
	fs.Bool(versionKey, false, "If true, print version and quit")

	// Config File:
	fs.String(configFileKey, defaultString, "Specifies a config file")

	// NetworkID:
	fs.String(networkNameKey, defaultNetworkName, "Network ID this node will connect to")

	// AVAX fees:
	fs.Uint64(txFeeKey, units.MilliAvax, "Transaction fee, in nAVAX")
	fs.Uint64(creationTxFeeKey, units.MilliAvax, "Transaction fee, in nAVAX, for transactions that create new state")

	// Uptime requirement:
	fs.Float64(uptimeRequirementKey, .6, "Fraction of time a validator must be online to receive rewards")

	// Minimum stake, in nAVAX, required to validate the primary network
	fs.Uint64(minValidatorStakeKey, 2*units.KiloAvax, "Minimum stake, in nAVAX, required to validate the primary network")

	// Maximum stake amount, in nAVAX, that can be staked and delegated to a
	// validator on the primary network
	fs.Uint64(maxValidatorStakeKey, 3*units.MegaAvax, "Maximum stake, in nAVAX, that can be placed on a validator on the primary network")

	// Minimum stake, in nAVAX, that can be delegated on the primary network
	fs.Uint64(minDelegatorStakeKey, 25*units.Avax, "Minimum stake, in nAVAX, that can be delegated on the primary network")

	fs.Uint64(minDelegatorFeeKey, 20000, "Minimum delegation fee, in the range [0, 1000000], that can be charged for delegation on the primary network")

	// Minimum staking duration
	fs.Duration(minStakeDurationKey, 24*time.Hour, "Minimum staking duration")

	// Maximum staking duration
	fs.Duration(maxStakeDurationKey, 365*24*time.Hour, "Maximum staking duration")

	// Stake minting period
	fs.Duration(stakeMintingPeriodKey, 365*24*time.Hour, "Consumption period of the staking function")

	// Assertions:
	fs.Bool(assertionsEnabledKey, true, "Turn on assertion execution")

	// Crypto:
	fs.Bool(signatureVerificationEnabledKey, true, "Turn on signature verification")

	// Database:
	fs.Bool(dbEnabledKey, true, "Turn on persistent storage")
	fs.String(dbDirKey, defaultString, "Database directory for Avalanche state")

	// IP:
	fs.String(publicIPKey, "", "Public IP of this node for P2P communication. If empty, try to discover with NAT. Ignored if dynamic-public-ip is non-empty.")

	// how often to update the dynamic IP and PnP/NAT-PMP IP and routing.
	fs.Duration(dynamicUpdateDurationKey, 5*time.Minute, "Dynamic IP and NAT Traversal update duration")

	fs.String(dynamicPublicIPResolverKey, "", "'ifconfigco' (alias 'ifconfig') or 'opendns' or 'ifconfigme'. By default does not do dynamic public IP updates. If non-empty, ignores public-ip argument.")

	// Incoming connection throttling
	// After we receive [conn-meter-max-conns] incoming connections from a given IP
	// in the last [conn-meter-reset-duration], we close all subsequent incoming connections
	// from the IP before upgrade.
	fs.Duration(connMeterResetDurationKey, 0*time.Second,
		"Upgrade at most [conn-meter-max-conns] connections from a given IP per [conn-meter-reset-duration]. "+
			"If [conn-meter-reset-duration] is 0, incoming connections are not rate-limited.")

	fs.Int(connMeterMaxConnsKey, 5,
		"Upgrade at most [conn-meter-max-conns] connections from a given IP per [conn-meter-reset-duration]. "+
			"If [conn-meter-reset-duration] is 0, incoming connections are not rate-limited.")

	// HTTP Server:
	fs.String(httpHostKey, "127.0.0.1", "Address of the HTTP server")
	fs.Uint(httpPortKey, 9650, "Port of the HTTP server")
	fs.Bool(httpsEnabledKey, false, "Upgrade the HTTP server to HTTPs")
	fs.String(httpsKeyFileKey, "", "TLS private key file for the HTTPs server")
	fs.String(httpsCertFileKey, "", "TLS certificate file for the HTTPs server")
	fs.Bool(apiAuthRequiredKey, false, "Require authorization token to call HTTP APIs")
	fs.String(apiAuthPasswordKey, "", "Password used to create/validate API authorization tokens. Can be changed via API call.")

	// Bootstrapping:
	fs.String(bootstrapIPsKey, defaultString, "Comma separated list of bootstrap peer ips to connect to. Example: 127.0.0.1:9630,127.0.0.1:9631")
	fs.String(bootstrapIDsKey, defaultString, "Comma separated list of bootstrap peer ids to connect to. Example: NodeID-JR4dVmy6ffUGAKCBDkyCbeZbyHQBeDsET,NodeID-8CrVPQZ4VSqgL8zTdvL14G8HqAfrBr4z")

	// Staking:
	fs.Uint(stakingPortKey, 9651, "Port of the consensus server")
	fs.Bool(stakingEnabledKey, true, "Enable staking. If enabled, Network TLS is required.")
	fs.Bool(p2pTLSEnabledKey, true, "Require TLS to authenticate network communication")
	fs.String(stakingKeyPathKey, defaultString, "TLS private key for staking")
	fs.String(stakingCertPathKey, defaultString, "TLS certificate for staking")
	fs.Uint64(stakingDisabledWeightKey, 1, "Weight to provide to each peer when staking is disabled")

	// Throttling:
	fs.Uint(maxNonStakerPendingMsgsKey, uint(router.DefaultMaxNonStakerPendingMsgs), "Maximum number of messages a non-staker is allowed to have pending.")
	fs.Float64(stakerMsgReservedKey, router.DefaultStakerPortion, "Reserve a portion of the chain message queue's space for stakers.")
	fs.Float64(stakerCPUReservedKey, router.DefaultStakerPortion, "Reserve a portion of the chain's CPU time for stakers.")
	fs.Uint(maxPendingMsgsKey, 1024, "Maximum number of pending messages. Messages after this will be dropped.")

	// Network Timeouts:
	fs.Duration(networkInitialTimeoutKey, 5*time.Second, "Initial timeout value of the adaptive timeout manager, in nanoseconds.")
	fs.Duration(networkMinimumTimeoutKey, 500*time.Millisecond, "Minimum timeout value of the adaptive timeout manager, in nanoseconds.")
	fs.Duration(networkMaximumTimeoutKey, 10*time.Second, "Maximum timeout value of the adaptive timeout manager, in nanoseconds.")
	fs.Duration(networkTimeoutIncreaseKey, 60*time.Millisecond, "Increase of network timeout after a failed request, in nanoseconds.")
	fs.Duration(networkTimeoutReductionKey, 12*time.Millisecond, "Decrease of network timeout after a successful request, in nanoseconds.")
	fs.Uint(sendQueueSizeKey, 1<<10, "Max number of messages waiting to be sent to peers.")

	// Benchlist Parameters:
	fs.Int(benchlistFailThresholdKey, 10, "Number of consecutive failed queries before benchlisting a node.")
	fs.Bool(benchlistPeerSummaryEnabledKey, false, "Enables peer specific query latency metrics.")
	fs.Duration(benchlistDurationKey, time.Hour, "Amount of time a peer is benchlisted after surpassing the threshold.")
	fs.Duration(benchlistMinFailingDurationKey, 5*time.Minute, "Minimum amount of time messages to a peer must be failing before the peer is benched.")

	// Plugins:
	fs.String(pluginDirKey, defaultString, "Plugin directory for Avalanche VMs")

	// Logging:
	fs.String(logsDirKey, "", "Logging directory for Avalanche")
	fs.String(logLevelKey, "info", "The log level. Should be one of {verbo, debug, info, warn, error, fatal, off}")
	fs.String(logDisplayLevelKey, "", "The log display level. If left blank, will inherit the value of log-level. Otherwise, should be one of {verbo, debug, info, warn, error, fatal, off}")
	fs.String(logDisplayHighlightKey, "auto", "Whether to color/highlight display logs. Default highlights when the output is a terminal. Otherwise, should be one of {auto, plain, colors}")

	fs.Int(snowSampleSizeKey, 20, "Number of nodes to query for each network poll")
	fs.Int(snowQuorumSizeKey, 14, "Alpha value to use for required number positive results")
	fs.Int(snowVirtuousCommitThresholdKey, 15, "Beta value to use for virtuous transactions")
	fs.Int(snowRogueCommitThresholdKey, 30, "Beta value to use for rogue transactions")
	fs.Int(snowAvalancheNumParentsKey, 5, "Number of vertexes for reference from each new vertex")
	fs.Int(snowAvalancheBatchSizeKey, 30, "Number of operations to batch in each new vertex")
	fs.Int(snowConcurrentRepollsKey, 4, "Minimum number of concurrent polls for finalizing consensus")
	fs.Int64(snowEpochFirstTransition, 1609873200, "Unix timestamp of the first epoch transaction, in seconds. Defaults to 1/5/2021 @ 7:00pm (UTC)")
	fs.Duration(snowEpochDuration, 6*time.Hour, "Duration of each epoch")

	// Enable/Disable APIs:
	fs.Bool(adminAPIEnabledKey, false, "If true, this node exposes the Admin API")
	fs.Bool(infoAPIEnabledKey, true, "If true, this node exposes the Info API")
	fs.Bool(keystoreAPIEnabledKey, true, "If true, this node exposes the Keystore API")
	fs.Bool(metricsAPIEnabledKey, true, "If true, this node exposes the Metrics API")
	fs.Bool(healthAPIEnabledKey, true, "If true, this node exposes the Health API")
	fs.Bool(ipcAPIEnabledKey, false, "If true, IPCs can be opened")

	// Throughput Server
	fs.Uint(xputServerPortKey, 9652, "Port of the deprecated throughput test server")
	fs.Bool(xputServerEnabledKey, false, "If true, throughput test server is created")

	// IPC
	fs.String(ipcsChainIDsKey, "", "Comma separated list of chain ids to add to the IPC engine. Example: 11111111111111111111111111111111LpoYY,4R5p2RXDGLqaifZE4hHWH9owe34pfoBULn1DrQTWivjg8o4aH")
	fs.String(ipcsPathKey, defaultString, "The directory (Unix) or named pipe name prefix (Windows) for IPC sockets")

	// Router Configuration:
	fs.Duration(consensusGossipFrequencyKey, 10*time.Second, "Frequency of gossiping accepted frontiers.")
	fs.Duration(consensusShutdownTimeoutKey, 5*time.Second, "Timeout before killing an unresponsive chain.")

	// Restart on disconnect configuration:
	fs.Duration(disconnectedCheckFreqKey, 10*time.Second, "How often the node checks if it is connected to any peers. "+
		"See [restart-on-disconnected]. If 0, node will not restart due to disconnection.")
	fs.Duration(disconnectedRestartTimeoutKey, 1*time.Minute, "If [restart-on-disconnected], node restarts if not connected to any peers for this amount of time. "+
		"If 0, node will not restart due to disconnection.")
	fs.Bool(restartOnDisconnectedKey, false, "If true, this node will restart if it is not connected to any peers for [disconnected-restart-timeout].")

	// File Descriptor Limit
	fs.Uint64(fdLimitKey, ulimit.DefaultFDLimit, "Attempts to raise the process file descriptor limit to at least this value.")

	// Subnet Whitelist
	fs.String(whitelistedSubnetsKey, "", "Whitelist of subnets to validate.")

	// Coreth Config
	fs.String(corethConfigKey, defaultString, "Specifies config to pass into coreth")

	return fs
}

// getViper returns the viper environment from parsing config file from default search paths
// and any parsed command line flags
func getViper() (*viper.Viper, error) {
	v := viper.New()

	fs := avalancheFlagSet()
	pflag.CommandLine.AddGoFlagSet(fs)
	pflag.Parse()
	if err := v.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}

	if configFile := v.GetString(configFileKey); configFile != defaultString {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	return v, nil
}

// setNodeConfig sets attributes on [Config] based on the values
// defined in the [viper] environment
func setNodeConfig(v *viper.Viper) error {
	// Consensus Parameters
	Config.ConsensusParams.K = v.GetInt(snowSampleSizeKey)
	Config.ConsensusParams.Alpha = v.GetInt(snowQuorumSizeKey)
	Config.ConsensusParams.BetaVirtuous = v.GetInt(snowVirtuousCommitThresholdKey)
	Config.ConsensusParams.BetaRogue = v.GetInt(snowRogueCommitThresholdKey)
	Config.ConsensusParams.Parents = v.GetInt(snowAvalancheNumParentsKey)
	Config.ConsensusParams.BatchSize = v.GetInt(snowAvalancheBatchSizeKey)
	Config.ConsensusParams.ConcurrentRepolls = v.GetInt(snowConcurrentRepollsKey)

	Config.ConsensusGossipFrequency = v.GetDuration(consensusGossipFrequencyKey)
	Config.ConsensusShutdownTimeout = v.GetDuration(consensusShutdownTimeoutKey)

	// Logging:
	loggingConfig, err := logging.DefaultConfig()
	if err != nil {
		return err
	}
	logsDir := v.GetString(logsDirKey)
	if logsDir != "" {
		loggingConfig.Directory = logsDir
	}
	loggingConfig.LogLevel, err = logging.ToLevel(v.GetString(logLevelKey))
	if err != nil {
		return err
	}

	logDisplayLevel := v.GetString(logDisplayLevelKey)
	if logDisplayLevel == "" {
		logDisplayLevel = v.GetString(logLevelKey)
	}
	displayLevel, err := logging.ToLevel(logDisplayLevel)
	if err != nil {
		return err
	}
	loggingConfig.DisplayLevel = displayLevel

	loggingConfig.DisplayHighlight, err = logging.ToHighlight(v.GetString(logDisplayHighlightKey), os.Stdout.Fd())
	if err != nil {
		return err
	}

	Config.LoggingConfig = loggingConfig

	// NetworkID
	networkID, err := constants.NetworkID(v.GetString(networkNameKey))
	if err != nil {
		return err
	}
	Config.NetworkID = networkID

	// DB:
	if v.GetBool(dbEnabledKey) {
		dbDir := v.GetString(dbDirKey)
		if dbDir == defaultString {
			dbDir = defaultDbDir
		}
		dbDir = os.ExpandEnv(dbDir) // parse any env variables
		dbPath := path.Join(dbDir, constants.NetworkName(Config.NetworkID), dbVersion)
		db, err := leveldb.New(dbPath, 0, 0, 0)
		if err != nil {
			return fmt.Errorf("couldn't create db at %s: %w", dbPath, err)
		}
		Config.DB = db
	} else {
		Config.DB = memdb.New()
	}

	// IP Configuration
	// Resolves our public IP, or does nothing
	Config.DynamicPublicIPResolver = dynamicip.NewResolver(v.GetString(dynamicPublicIPResolverKey))

	var ip net.IP
	publicIP := v.GetString(publicIPKey)
	switch {
	case Config.DynamicPublicIPResolver.IsResolver():
		// User specified to use dynamic IP resolution; don't use NAT traversal
		Config.Nat = nat.NewNoRouter()
		ip, err = dynamicip.FetchExternalIP(Config.DynamicPublicIPResolver)
		if err != nil {
			return fmt.Errorf("dynamic ip address fetch failed: %s", err)
		}

	case publicIP == "":
		// User didn't specify a public IP to use; try with NAT traversal
		Config.AttemptedNATTraversal = true
		Config.Nat = nat.GetRouter()
		ip, err = Config.Nat.ExternalIP()
		if err != nil {
			ip = net.IPv4zero // Couldn't get my IP...set to 0.0.0.0
		}
	default:
		// User specified a public IP to use; don't use NAT
		Config.Nat = nat.NewNoRouter()
		ip = net.ParseIP(publicIP)
	}

	if ip == nil {
		return fmt.Errorf("invalid IP Address %s", publicIP)
	}

	Config.StakingIP = utils.NewDynamicIPDesc(
		ip,
		uint16(v.GetUint(stakingPortKey)),
	)

	Config.DynamicUpdateDuration = v.GetDuration(dynamicUpdateDurationKey)

	Config.ConnMeterResetDuration = v.GetDuration(connMeterResetDurationKey)
	Config.ConnMeterMaxConns = v.GetInt(connMeterMaxConnsKey)

	// Staking:
	Config.EnableStaking = v.GetBool(stakingEnabledKey)
	Config.EnableP2PTLS = v.GetBool(p2pTLSEnabledKey)
	Config.StakingKeyFile = v.GetString(stakingKeyPathKey)
	Config.StakingCertFile = v.GetString(stakingCertPathKey)
	Config.DisabledStakingWeight = v.GetUint64(stakingDisabledWeightKey)

	Config.MinStakeDuration = v.GetDuration(minStakeDurationKey)
	Config.MaxStakeDuration = v.GetDuration(maxStakeDurationKey)
	Config.StakeMintingPeriod = v.GetDuration(stakeMintingPeriodKey)

	stakingKeyPath := v.GetString(stakingKeyPathKey)
	if stakingKeyPath == defaultString {
		Config.StakingKeyFile = defaultStakingKeyPath
	} else {
		Config.StakingKeyFile = stakingKeyPath
	}
	stakingCertPath := v.GetString(stakingCertPathKey)
	if stakingCertPath == defaultString {
		Config.StakingCertFile = defaultStakingCertPath
	} else {
		Config.StakingCertFile = stakingCertPath
	}

	// parse any env variables
	Config.StakingCertFile = os.ExpandEnv(Config.StakingCertFile)
	Config.StakingKeyFile = os.ExpandEnv(Config.StakingKeyFile)
	switch {
	// If staking key/cert locations are specified but not found, error
	case Config.StakingKeyFile != defaultStakingKeyPath || Config.StakingCertFile != defaultStakingCertPath:
		if _, err := os.Stat(Config.StakingKeyFile); os.IsNotExist(err) {
			return fmt.Errorf("couldn't find staking key at %s", Config.StakingKeyFile)
		} else if _, err := os.Stat(Config.StakingCertFile); os.IsNotExist(err) {
			return fmt.Errorf("couldn't find staking certificate at %s", Config.StakingCertFile)
		}
	default:
		// Only creates staking key/cert if [stakingKeyPath] doesn't exist
		if err := staking.GenerateStakingKeyCert(Config.StakingKeyFile, Config.StakingCertFile); err != nil {
			return fmt.Errorf("couldn't generate staking key/cert: %w", err)
		}
	}

	// Bootstrapping:
	defaultBootstrapIPs, defaultBootstrapIDs := genesis.SampleBeacons(Config.NetworkID, 5)
	bootstrapIPs := v.GetString(bootstrapIPsKey)
	if bootstrapIPs == defaultString {
		bootstrapIPs = strings.Join(defaultBootstrapIPs, ",")
	}
	for _, ip := range strings.Split(bootstrapIPs, ",") {
		if ip != "" {
			addr, err := utils.ToIPDesc(ip)
			if err != nil {
				return fmt.Errorf("couldn't parse bootstrap ip %s: %w", ip, err)
			}
			Config.BootstrapPeers = append(Config.BootstrapPeers, &node.Peer{
				IP: addr,
			})
		}
	}

	bootstrapIDs := v.GetString(bootstrapIDsKey)
	if bootstrapIDs == defaultString {
		if bootstrapIPs == "" {
			bootstrapIDs = ""
		} else {
			bootstrapIDs = strings.Join(defaultBootstrapIDs, ",")
		}
	}

	if Config.EnableStaking && !Config.EnableP2PTLS {
		return errStakingRequiresTLS
	}

	if !Config.EnableStaking && Config.DisabledStakingWeight == 0 {
		return errInvalidStakerWeights
	}

	if Config.EnableP2PTLS {
		i := 0
		for _, id := range strings.Split(bootstrapIDs, ",") {
			if id != "" {
				peerID, err := ids.ShortFromPrefixedString(id, constants.NodeIDPrefix)
				if err != nil {
					return fmt.Errorf("couldn't parse bootstrap peer id: %w", err)
				}
				if len(Config.BootstrapPeers) <= i {
					return errBootstrapMismatch
				}
				Config.BootstrapPeers[i].ID = peerID
				i++
			}
		}
		if len(Config.BootstrapPeers) != i {
			return fmt.Errorf("more bootstrap IPs, %d, provided than bootstrap IDs, %d", len(Config.BootstrapPeers), i)
		}
	} else {
		for _, peer := range Config.BootstrapPeers {
			peer.ID = ids.ShortID(hashing.ComputeHash160Array([]byte(peer.IP.String())))
		}
	}

	Config.WhitelistedSubnets.Add(constants.PrimaryNetworkID)
	for _, subnet := range strings.Split(v.GetString(whitelistedSubnetsKey), ",") {
		if subnet != "" {
			subnetID, err := ids.FromString(subnet)
			if err != nil {
				return fmt.Errorf("couldn't parse subnetID %s: %w", subnet, err)
			}
			Config.WhitelistedSubnets.Add(subnetID)
		}
	}

	// Plugins
	pluginDir := v.GetString(pluginDirKey)
	if pluginDir == defaultString {
		Config.PluginDir = defaultPluginDirs[0]
	} else {
		Config.PluginDir = pluginDir
	}
	if _, err := os.Stat(Config.PluginDir); os.IsNotExist(err) {
		for _, dir := range defaultPluginDirs {
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				Config.PluginDir = dir
				break
			}
		}
	}

	// HTTP:
	Config.HTTPHost = v.GetString(httpHostKey)
	Config.HTTPPort = uint16(v.GetUint(httpPortKey))
	if Config.APIRequireAuthToken {
		if Config.APIAuthPassword == "" {
			return errors.New("api-auth-password must be provided if api-auth-required is true")
		}
		if !password.SufficientlyStrong(Config.APIAuthPassword, password.OK) {
			return errors.New("api-auth-password is not strong enough. Add more characters")
		}
	}

	Config.HTTPSEnabled = v.GetBool(httpsEnabledKey)
	Config.HTTPSKeyFile = v.GetString(httpsKeyFileKey)
	Config.HTTPSCertFile = v.GetString(httpsCertFileKey)

	// API Auth
	Config.APIRequireAuthToken = v.GetBool(apiAuthRequiredKey)
	Config.APIAuthPassword = v.GetString(apiAuthPasswordKey)

	// APIs
	Config.AdminAPIEnabled = v.GetBool(adminAPIEnabledKey)
	Config.InfoAPIEnabled = v.GetBool(infoAPIEnabledKey)
	Config.KeystoreAPIEnabled = v.GetBool(keystoreAPIEnabledKey)
	Config.MetricsAPIEnabled = v.GetBool(metricsAPIEnabledKey)
	Config.HealthAPIEnabled = v.GetBool(healthAPIEnabledKey)
	Config.IPCAPIEnabled = v.GetBool(ipcAPIEnabledKey)

	// Throughput:
	Config.ThroughputServerEnabled = v.GetBool(xputServerEnabledKey)
	Config.ThroughputPort = uint16(v.GetUint(xputServerPortKey))

	// Router used for consensus
	Config.ConsensusRouter = &router.ChainRouter{}

	// IPCs
	ipcsChainIDs := v.GetString(ipcsChainIDsKey)
	if ipcsChainIDs != "" {
		Config.IPCDefaultChainIDs = strings.Split(ipcsChainIDs, ",")
	}

	ipcsPath := v.GetString(ipcsPathKey)
	if ipcsPath == defaultString {
		Config.IPCPath = ipcs.DefaultBaseURL
	} else {
		Config.IPCPath = ipcsPath
	}

	// Throttling
	Config.MaxNonStakerPendingMsgs = v.GetUint32(maxNonStakerPendingMsgsKey)
	Config.StakerMSGPortion = v.GetFloat64(stakerMsgReservedKey)
	Config.StakerCPUPortion = v.GetFloat64(stakerCPUReservedKey)
	Config.SendQueueSize = v.GetUint32(sendQueueSizeKey)
	Config.MaxPendingMsgs = v.GetUint32(maxPendingMsgsKey)
	if Config.MaxPendingMsgs < Config.MaxNonStakerPendingMsgs {
		return errors.New("maximum pending messages must be >= maximum non-staker pending messages")
	}

	// Network Timeout
	Config.NetworkConfig.InitialTimeout = v.GetDuration(networkInitialTimeoutKey)
	Config.NetworkConfig.MinimumTimeout = v.GetDuration(networkMinimumTimeoutKey)
	Config.NetworkConfig.MaximumTimeout = v.GetDuration(networkMaximumTimeoutKey)
	Config.NetworkConfig.TimeoutInc = v.GetDuration(networkTimeoutIncreaseKey)
	Config.NetworkConfig.TimeoutDec = v.GetDuration(networkTimeoutReductionKey)

	if Config.NetworkConfig.MinimumTimeout < 1 {
		return errors.New("minimum timeout must be positive")
	}
	if Config.NetworkConfig.MinimumTimeout > Config.NetworkConfig.MaximumTimeout {
		return errors.New("maximum timeout can't be less than minimum timeout")
	}
	if Config.NetworkConfig.InitialTimeout < Config.NetworkConfig.MinimumTimeout ||
		Config.NetworkConfig.InitialTimeout > Config.NetworkConfig.MaximumTimeout {
		return errors.New("initial timeout should be in the range [minimumTimeout, maximumTimeout]")
	}
	if Config.NetworkConfig.TimeoutDec < 0 {
		return errors.New("timeout reduction can't be negative")
	}
	if Config.NetworkConfig.TimeoutInc < 0 {
		return errors.New("timeout increase can't be negative")
	}

	// Restart:
	Config.RestartOnDisconnected = v.GetBool(restartOnDisconnectedKey)
	Config.DisconnectedCheckFreq = v.GetDuration(disconnectedCheckFreqKey)
	Config.DisconnectedRestartTimeout = v.GetDuration(disconnectedRestartTimeoutKey)
	if Config.DisconnectedCheckFreq > Config.DisconnectedRestartTimeout {
		return fmt.Errorf("[%s] can't be greater than [%s]", disconnectedCheckFreqKey, disconnectedRestartTimeoutKey)
	}

	// Benchlist
	Config.BenchlistConfig.Threshold = v.GetInt(benchlistFailThresholdKey)
	Config.BenchlistConfig.PeerSummaryEnabled = v.GetBool(benchlistPeerSummaryEnabledKey)
	Config.BenchlistConfig.Duration = v.GetDuration(benchlistDurationKey)
	Config.BenchlistConfig.MinimumFailingDuration = v.GetDuration(benchlistMinFailingDurationKey)
	Config.BenchlistConfig.MaxPortion = (1.0 - (float64(Config.ConsensusParams.Alpha) / float64(Config.ConsensusParams.K))) / 3.0

	if Config.ConsensusGossipFrequency < 0 {
		return errors.New("gossip frequency can't be negative")
	}
	if Config.ConsensusShutdownTimeout < 0 {
		return errors.New("gossip frequency can't be negative")
	}

	// File Descriptor Limit
	fdLimit := v.GetUint64(fdLimitKey)
	if err := ulimit.Set(fdLimit); err != nil {
		return fmt.Errorf("failed to set fd limit correctly due to: %w", err)
	}

	// Network Parameters
	if networkID != constants.MainnetID && networkID != constants.FujiID {
		txFee := v.GetUint64(txFeeKey)
		creationTxFee := v.GetUint64(creationTxFeeKey)
		uptimeRequirement := v.GetFloat64(uptimeRequirementKey)
		Config.TxFee = txFee
		Config.CreationTxFee = creationTxFee
		Config.UptimeRequirement = uptimeRequirement

		minValidatorStake := v.GetUint64(minValidatorStakeKey)
		maxValidatorStake := v.GetUint64(maxValidatorStakeKey)
		minDelegatorStake := v.GetUint64(minDelegatorStakeKey)
		minDelegationFee := v.GetUint64(minDelegatorFeeKey)
		if minValidatorStake > maxValidatorStake {
			return errors.New("minimum validator stake can't be greater than maximum validator stake")
		}

		Config.MinValidatorStake = minValidatorStake
		Config.MaxValidatorStake = maxValidatorStake
		Config.MinDelegatorStake = minDelegatorStake

		if minDelegationFee > 1000000 {
			return errors.New("delegation fee must be in the range [0, 1000000]")
		}
		Config.MinDelegationFee = uint32(minDelegationFee)

		if Config.MinStakeDuration == 0 {
			return errors.New("min stake duration can't be zero")
		}
		if Config.MaxStakeDuration < Config.MinStakeDuration {
			return errors.New("max stake duration can't be less than min stake duration")
		}
		if Config.StakeMintingPeriod < Config.MaxStakeDuration {
			return errors.New("stake minting period can't be less than max stake duration")
		}

		Config.EpochFirstTransition = time.Unix(v.GetInt64(snowEpochFirstTransition), 0)
		Config.EpochDuration = v.GetDuration(snowEpochDuration)
	} else {
		Config.Params = *genesis.GetParams(networkID)
	}

	// Assertions
	Config.EnableAssertions = v.GetBool(assertionsEnabledKey)

	// Crypto
	Config.EnableCrypto = v.GetBool(signatureVerificationEnabledKey)

	// Coreth Plugin
	corethConfigString := v.GetString(corethConfigKey)
	if corethConfigString != defaultString {
		corethConfigValue := v.Get(corethConfigKey)
		switch value := corethConfigValue.(type) {
		case string:
			corethConfigString = value
		default:
			corethConfigBytes, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("couldn't parse coreth config: %w", err)
			}
			corethConfigString = string(corethConfigBytes)
		}
	}
	Config.CorethConfig = corethConfigString

	return nil
}

func parseViper() error {
	v, err := getViper()
	if err != nil {
		return err
	}

	if v.GetBool(versionKey) {
		format := "%s ["
		args := []interface{}{
			node.Version,
		}

		networkID, err := constants.NetworkID(v.GetString(networkNameKey))
		if err != nil {
			return err
		}
		networkGeneration := constants.NetworkName(networkID)
		if networkID == constants.MainnetID {
			format += "network=%s"
		} else {
			format += "network=testnet/%s"
		}
		args = append(args, networkGeneration)

		format += ", database=%s"
		args = append(args, dbVersion)

		if GitCommit != "" {
			format += ", commit=%s"
			args = append(args, GitCommit)
		}

		format += "]\n"

		fmt.Printf(format, args...)
		os.Exit(0)
	}

	return setNodeConfig(v)
}
