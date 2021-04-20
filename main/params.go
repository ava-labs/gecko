// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/kardianos/osext"

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
	defaultDataDir         = filepath.Join(homeDir, prefixedAppName)
	defaultDbDir           = filepath.Join(defaultDataDir, "db")
	defaultStakingKeyPath  = filepath.Join(defaultDataDir, "staking", "staker.key")
	defaultStakingCertPath = filepath.Join(defaultDataDir, "staking", "staker.crt")
	defaultPluginDirs      = []string{
		filepath.Join(".", "build", "plugins"),
		filepath.Join(".", "plugins"),
		filepath.Join("/", "usr", "local", "lib", constants.AppName),
		filepath.Join(defaultDataDir, "plugins"),
	}
	// GitCommit should be optionally set at compile time.
	GitCommit string
)

func init() {
	folderPath, err := osext.ExecutableFolder()
	if err == nil {
		defaultPluginDirs = append(defaultPluginDirs, filepath.Join(folderPath, "plugins"))
	}
}

var (
	errBootstrapMismatch    = errors.New("more bootstrap IDs provided than bootstrap IPs")
	errStakingRequiresTLS   = errors.New("if staking is enabled, network TLS must also be enabled")
	errInvalidStakerWeights = errors.New("staking weights must be positive")
)

// avalancheFlagSet returns the complete set of flags for avalanchego
func avalancheFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(constants.AppName, flag.ContinueOnError)

	// If true, print the version and quit.
	fs.Bool(versionKey, false, "If true, print version and quit")

	// System
	fs.Uint64(fdLimitKey, ulimit.DefaultFDLimit, "Attempts to raise the process file descriptor limit to at least this value.")

	// Config
	fs.String(configFileKey, defaultString, "Specifies a config file")
	// Genesis Config File
	fs.String(genesisConfigFileKey, "", "Specifies a genesis config file (ignored when running standard networks)")
	// Plugins
	fs.String(pluginDirKey, defaultString, "Plugin directory for Avalanche VMs")
	// Network ID
	fs.String(networkNameKey, defaultNetworkName, "Network ID this node will connect to")
	// AVAX fees
	fs.Uint64(txFeeKey, units.MilliAvax, "Transaction fee, in nAVAX")
	fs.Uint64(creationTxFeeKey, units.MilliAvax, "Transaction fee, in nAVAX, for transactions that create new state")
	// Database
	fs.Bool(dbEnabledKey, true, "Turn on persistent storage")
	fs.String(dbPathKey, defaultDbDir, "Path to database directory")
	// Coreth Config
	fs.String(corethConfigKey, defaultString, "Specifies config to pass into coreth")
	// Logging
	fs.String(logsDirKey, "", "Logging directory for Avalanche")
	fs.String(logLevelKey, "info", "The log level. Should be one of {verbo, debug, info, warn, error, fatal, off}")
	fs.String(logDisplayLevelKey, "", "The log display level. If left blank, will inherit the value of log-level. Otherwise, should be one of {verbo, debug, info, warn, error, fatal, off}")
	fs.String(logDisplayHighlightKey, "auto", "Whether to color/highlight display logs. Default highlights when the output is a terminal. Otherwise, should be one of {auto, plain, colors}")
	// Assertions
	fs.Bool(assertionsEnabledKey, true, "Turn on assertion execution")
	// Signature Verification
	fs.Bool(signatureVerificationEnabledKey, true, "Turn on signature verification")

	// Networking
	// Public IP Resolution
	fs.String(publicIPKey, "", "Public IP of this node for P2P communication. If empty, try to discover with NAT. Ignored if dynamic-public-ip is non-empty.")
	fs.Duration(dynamicUpdateDurationKey, 5*time.Minute, "Dynamic IP and NAT Traversal update duration")
	fs.String(dynamicPublicIPResolverKey, "", "'ifconfigco' (alias 'ifconfig') or 'opendns' or 'ifconfigme'. By default does not do dynamic public IP updates. If non-empty, ignores public-ip argument.")
	// Incoming Connection Throttling
	// After we receive [conn-meter-max-conns] incoming connections from a given IP
	// in the last [conn-meter-reset-duration], we close all subsequent incoming connections
	// from the IP before upgrade.
	fs.Duration(connMeterResetDurationKey, 0*time.Second,
		"Upgrade at most [conn-meter-max-conns] connections from a given IP per [conn-meter-reset-duration]. "+
			"If [conn-meter-reset-duration] is 0, incoming connections are not rate-limited.")
	fs.Int(connMeterMaxConnsKey, 5,
		"Upgrade at most [conn-meter-max-conns] connections from a given IP per [conn-meter-reset-duration]. "+
			"If [conn-meter-reset-duration] is 0, incoming connections are not rate-limited.")
	// Timeouts
	fs.Duration(networkInitialTimeoutKey, 5*time.Second, "Initial timeout value of the adaptive timeout manager.")
	fs.Duration(networkMinimumTimeoutKey, 2*time.Second, "Minimum timeout value of the adaptive timeout manager.")
	fs.Duration(networkMaximumTimeoutKey, 10*time.Second, "Maximum timeout value of the adaptive timeout manager.")
	fs.Duration(networkTimeoutHalflifeKey, 5*time.Minute, "Halflife of average network response time. Higher value --> network timeout is less volatile. Can't be 0.")
	fs.Float64(networkTimeoutCoefficientKey, 2, "Multiplied by average network response time to get the network timeout. Must be >= 1.")
	fs.Uint(sendQueueSizeKey, 4096, "Max number of messages waiting to be sent to peers.")
	// Restart on Disconnect
	fs.Duration(disconnectedCheckFreqKey, 10*time.Second, "How often the node checks if it is connected to any peers. "+
		"See [restart-on-disconnected]. If 0, node will not restart due to disconnection.")
	fs.Duration(disconnectedRestartTimeoutKey, 1*time.Minute, "If [restart-on-disconnected], node restarts if not connected to any peers for this amount of time. "+
		"If 0, node will not restart due to disconnection.")
	fs.Bool(restartOnDisconnectedKey, false, "If true, this node will restart if it is not connected to any peers for [disconnected-restart-timeout].")
	// Peer alias configuration
	fs.Duration(peerAliasTimeoutKey, 10*time.Minute, "How often the node will attempt to connect "+
		"to an IP address previously associated with a peer (i.e. a peer alias).")
	// Benchlist
	fs.Int(benchlistFailThresholdKey, 10, "Number of consecutive failed queries before benchlisting a node.")
	fs.Bool(benchlistPeerSummaryEnabledKey, false, "Enables peer specific query latency metrics.")
	fs.Duration(benchlistDurationKey, 30*time.Minute, "Max amount of time a peer is benchlisted after surpassing the threshold.")
	fs.Duration(benchlistMinFailingDurationKey, 5*time.Minute, "Minimum amount of time messages to a peer must be failing before the peer is benched.")
	// Router
	fs.Uint(maxNonStakerPendingMsgsKey, uint(router.DefaultMaxNonStakerPendingMsgs), "Maximum number of messages a non-staker is allowed to have pending.")
	fs.Float64(stakerMsgReservedKey, router.DefaultStakerPortion, "Reserve a portion of the chain message queue's space for stakers.")
	fs.Float64(stakerCPUReservedKey, router.DefaultStakerPortion, "Reserve a portion of the chain's CPU time for stakers.")
	fs.Uint(maxPendingMsgsKey, 4096, "Maximum number of pending messages. Messages after this will be dropped.")
	fs.Duration(consensusGossipFrequencyKey, 10*time.Second, "Frequency of gossiping accepted frontiers.")
	fs.Duration(consensusShutdownTimeoutKey, 5*time.Second, "Timeout before killing an unresponsive chain.")

	// HTTP API
	fs.String(httpHostKey, "127.0.0.1", "Address of the HTTP server")
	fs.Uint(httpPortKey, 9650, "Port of the HTTP server")
	fs.Bool(httpsEnabledKey, false, "Upgrade the HTTP server to HTTPs")
	fs.String(httpsKeyFileKey, "", "TLS private key file for the HTTPs server")
	fs.String(httpsCertFileKey, "", "TLS certificate file for the HTTPs server")
	fs.String(httpAllowedOrigins, "*", "Origins to allow on the HTTP port. Defaults to * which allows all origins. Example: https://*.avax.network https://*.avax-test.network")
	fs.Bool(apiAuthRequiredKey, false, "Require authorization token to call HTTP APIs")
	fs.String(apiAuthPasswordFileKey, "", "Password file used to initially create/validate API authorization tokens. Leading and trailing whitespace is removed from the password. Can be changed via API call.")
	// Enable/Disable APIs
	fs.Bool(adminAPIEnabledKey, false, "If true, this node exposes the Admin API")
	fs.Bool(infoAPIEnabledKey, true, "If true, this node exposes the Info API")
	fs.Bool(keystoreAPIEnabledKey, true, "If true, this node exposes the Keystore API")
	fs.Bool(metricsAPIEnabledKey, true, "If true, this node exposes the Metrics API")
	fs.Bool(healthAPIEnabledKey, true, "If true, this node exposes the Health API")
	fs.Bool(ipcAPIEnabledKey, false, "If true, IPCs can be opened")
	// Throughput Server (deprecated)
	fs.Uint(xputServerPortKey, 9652, "Port of the deprecated throughput test server")
	fs.Bool(xputServerEnabledKey, false, "If true, throughput test server is created")
	// Health
	fs.Duration(healthCheckFreqKey, 30*time.Second, "Time between health checks")
	fs.Duration(healthCheckAveragerHalflifeKey, 10*time.Second, "Halflife of averager when calculating a running average in a health check")
	// Network Layer Health
	fs.Duration(networkHealthMaxTimeSinceMsgSentKey, time.Minute, "Network layer returns unhealthy if haven't sent a message for at least this much time")
	fs.Duration(networkHealthMaxTimeSinceMsgReceivedKey, time.Minute, "Network layer returns unhealthy if haven't received a message for at least this much time")
	fs.Float64(networkHealthMaxPortionSendQueueFillKey, 0.9, "Network layer returns unhealthy if more than this portion of the pending send queue is full")
	fs.Uint(networkHealthMinPeersKey, 1, "Network layer returns unhealthy if connected to less than this many peers")
	fs.Float64(networkHealthMaxSendFailRateKey, .9, "Network layer reports unhealthy if more than this portion of attempted message sends fail")
	// Router Health
	fs.Float64(routerHealthMaxDropRateKey, 1, "Node reports unhealthy if the router drops more than this portion of messages.")
	fs.Uint(routerHealthMaxOutstandingRequestsKey, 1024, "Node reports unhealthy if there are more than this many outstanding consensus requests (Get, PullQuery, etc.) over all chains")
	fs.Duration(networkHealthMaxOutstandingDurationKey, 5*time.Minute, "Node reports unhealthy if there has been a request outstanding for this duration")

	// Staking
	fs.Uint(stakingPortKey, 9651, "Port of the consensus server")
	fs.Bool(stakingEnabledKey, true, "Enable staking. If enabled, Network TLS is required.")
	fs.Bool(p2pTLSEnabledKey, true, "Require TLS to authenticate network communication")
	fs.String(stakingKeyPathKey, defaultString, "Path to the TLS private key for staking")
	fs.String(stakingCertPathKey, defaultString, "Path to the TLS certificate for staking")
	fs.Uint64(stakingDisabledWeightKey, 1, "Weight to provide to each peer when staking is disabled")
	// Uptime Requirement
	fs.Float64(uptimeRequirementKey, .6, "Fraction of time a validator must be online to receive rewards")
	// Minimum Stake required to validate the Primary Network
	fs.Uint64(minValidatorStakeKey, 2*units.KiloAvax, "Minimum stake, in nAVAX, required to validate the primary network")
	// Maximum Stake that can be staked and delegated to a validator on the Primary Network
	fs.Uint64(maxValidatorStakeKey, 3*units.MegaAvax, "Maximum stake, in nAVAX, that can be placed on a validator on the primary network")
	// Minimum Stake that can be delegated on the Primary Network
	fs.Uint64(minDelegatorStakeKey, 25*units.Avax, "Minimum stake, in nAVAX, that can be delegated on the primary network")
	fs.Uint64(minDelegatorFeeKey, 20000, "Minimum delegation fee, in the range [0, 1000000], that can be charged for delegation on the primary network")
	// Minimum Stake Duration
	fs.Duration(minStakeDurationKey, 24*time.Hour, "Minimum staking duration")
	// Maximum Stake Duration
	fs.Duration(maxStakeDurationKey, 365*24*time.Hour, "Maximum staking duration")
	// Stake Minting Period
	fs.Duration(stakeMintingPeriodKey, 365*24*time.Hour, "Consumption period of the staking function")
	// Subnets
	fs.String(whitelistedSubnetsKey, "", "Whitelist of subnets to validate.")
	// Bootstrapping
	fs.String(bootstrapIPsKey, defaultString, "Comma separated list of bootstrap peer ips to connect to. Example: 127.0.0.1:9630,127.0.0.1:9631")
	fs.String(bootstrapIDsKey, defaultString, "Comma separated list of bootstrap peer ids to connect to. Example: NodeID-JR4dVmy6ffUGAKCBDkyCbeZbyHQBeDsET,NodeID-8CrVPQZ4VSqgL8zTdvL14G8HqAfrBr4z")
	fs.Bool(retryBootstrap, true, "Specifies whether bootstrap should be retried")
	fs.Int(retryBootstrapMaxAttempts, 50, "Specifies how many times bootstrap should be retried")

	// Consensus
	fs.Int(snowSampleSizeKey, 20, "Number of nodes to query for each network poll")
	fs.Int(snowQuorumSizeKey, 14, "Alpha value to use for required number positive results")
	fs.Int(snowVirtuousCommitThresholdKey, 15, "Beta value to use for virtuous transactions")
	fs.Int(snowRogueCommitThresholdKey, 20, "Beta value to use for rogue transactions")
	fs.Int(snowAvalancheNumParentsKey, 5, "Number of vertexes for reference from each new vertex")
	fs.Int(snowAvalancheBatchSizeKey, 30, "Number of operations to batch in each new vertex")
	fs.Int(snowConcurrentRepollsKey, 4, "Minimum number of concurrent polls for finalizing consensus")
	fs.Int(snowOptimalProcessingKey, 50, "Optimal number of processing vertices in consensus")
	fs.Int(snowMaxProcessingKey, 1024, "Maximum number of processing items to be considered healthy")
	fs.Duration(snowMaxTimeProcessingKey, 2*time.Minute, "Maximum amount of time an item should be processing and still be healthy")
	fs.Int64(snowEpochFirstTransition, 1607626800, "Unix timestamp of the first epoch transaction, in seconds. Defaults to 12/10/2020 @ 7:00pm (UTC)")
	fs.Duration(snowEpochDuration, 6*time.Hour, "Duration of each epoch")

	// IPC
	fs.String(ipcsChainIDsKey, "", "Comma separated list of chain ids to add to the IPC engine. Example: 11111111111111111111111111111111LpoYY,4R5p2RXDGLqaifZE4hHWH9owe34pfoBULn1DrQTWivjg8o4aH")
	fs.String(ipcsPathKey, defaultString, "The directory (Unix) or named pipe name prefix (Windows) for IPC sockets")

	// Indexer
	// TODO handle the below line better
	fs.Bool(indexEnabledKey, false, "If true, index all accepted containers and transactions and expose them via an API")
	fs.Bool(indexAllowIncompleteKey, false, "If true, allow running the node in such a way that could cause an index to miss transactions. Ignored if index is disabled.")

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
	Config.ConsensusParams.OptimalProcessing = v.GetInt(snowOptimalProcessingKey)
	Config.ConsensusParams.MaxOutstandingItems = v.GetInt(snowMaxProcessingKey)
	Config.ConsensusParams.MaxItemProcessingTime = v.GetDuration(snowMaxTimeProcessingKey)
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
	Config.DBEnabled = v.GetBool(dbEnabledKey)
	Config.DBPath = os.ExpandEnv(v.GetString(dbPathKey))
	if Config.DBPath == defaultString {
		Config.DBPath = defaultDbDir
	}
	Config.DBPath = path.Join(Config.DBPath, constants.NetworkName(Config.NetworkID), dbVersion)

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

	Config.HTTPSEnabled = v.GetBool(httpsEnabledKey)
	Config.HTTPSKeyFile = v.GetString(httpsKeyFileKey)
	Config.HTTPSCertFile = v.GetString(httpsCertFileKey)
	Config.APIAllowedOrigins = v.GetStringSlice(httpAllowedOrigins)

	// API Auth
	Config.APIRequireAuthToken = v.GetBool(apiAuthRequiredKey)
	if Config.APIRequireAuthToken {
		passwordFile := v.GetString(apiAuthPasswordFileKey)
		pwBytes, err := ioutil.ReadFile(passwordFile)
		if err != nil {
			return fmt.Errorf("api-auth-password-file %q failed to be read with: %w", passwordFile, err)
		}
		Config.APIAuthPassword = strings.TrimSpace(string(pwBytes))
		if !password.SufficientlyStrong(Config.APIAuthPassword, password.OK) {
			return errors.New("api-auth-password is not strong enough")
		}
	}

	// APIs
	Config.AdminAPIEnabled = v.GetBool(adminAPIEnabledKey)
	Config.InfoAPIEnabled = v.GetBool(infoAPIEnabledKey)
	Config.KeystoreAPIEnabled = v.GetBool(keystoreAPIEnabledKey)
	Config.MetricsAPIEnabled = v.GetBool(metricsAPIEnabledKey)
	Config.HealthAPIEnabled = v.GetBool(healthAPIEnabledKey)
	Config.IPCAPIEnabled = v.GetBool(ipcAPIEnabledKey)
	Config.IndexAPIEnabled = v.GetBool(indexEnabledKey)

	// Throughput:
	Config.ThroughputServerEnabled = v.GetBool(xputServerEnabledKey)
	Config.ThroughputPort = uint16(v.GetUint(xputServerPortKey))

	// Halflife of continuous averager used in health checks
	healthCheckAveragerHalflife := v.GetDuration(healthCheckAveragerHalflifeKey)
	if healthCheckAveragerHalflife <= 0 {
		return fmt.Errorf("%s must be positive", healthCheckAveragerHalflifeKey)
	}

	// Router
	Config.ConsensusRouter = &router.ChainRouter{}
	Config.RouterHealthConfig.MaxDropRate = v.GetFloat64(routerHealthMaxDropRateKey)
	Config.RouterHealthConfig.MaxOutstandingRequests = int(v.GetUint(routerHealthMaxOutstandingRequestsKey))
	Config.RouterHealthConfig.MaxOutstandingDuration = v.GetDuration(networkHealthMaxOutstandingDurationKey)
	Config.RouterHealthConfig.MaxRunTimeRequests = v.GetDuration(networkMaximumTimeoutKey)
	Config.RouterHealthConfig.MaxDropRateHalflife = healthCheckAveragerHalflife
	switch {
	case Config.RouterHealthConfig.MaxDropRate < 0 || Config.RouterHealthConfig.MaxDropRate > 1:
		return fmt.Errorf("%s must be in [0,1]", routerHealthMaxDropRateKey)
	case Config.RouterHealthConfig.MaxOutstandingDuration <= 0:
		return fmt.Errorf("%s must be positive", networkHealthMaxOutstandingDurationKey)
	}

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

	// Health
	Config.HealthCheckFreq = v.GetDuration(healthCheckFreqKey)
	// Network Health Check
	Config.NetworkHealthConfig.MaxTimeSinceMsgSent = v.GetDuration(networkHealthMaxTimeSinceMsgSentKey)
	Config.NetworkHealthConfig.MaxTimeSinceMsgReceived = v.GetDuration(networkHealthMaxTimeSinceMsgReceivedKey)
	Config.NetworkHealthConfig.MaxPortionSendQueueBytesFull = v.GetFloat64(networkHealthMaxPortionSendQueueFillKey)
	Config.NetworkHealthConfig.MinConnectedPeers = v.GetUint(networkHealthMinPeersKey)
	Config.NetworkHealthConfig.MaxSendFailRate = v.GetFloat64(networkHealthMaxSendFailRateKey)
	Config.NetworkHealthConfig.MaxSendFailRateHalflife = healthCheckAveragerHalflife
	switch {
	case Config.NetworkHealthConfig.MaxTimeSinceMsgSent < 0:
		return fmt.Errorf("%s must be > 0", networkHealthMaxTimeSinceMsgSentKey)
	case Config.NetworkHealthConfig.MaxTimeSinceMsgReceived < 0:
		return fmt.Errorf("%s must be > 0", networkHealthMaxTimeSinceMsgReceivedKey)
	case Config.NetworkHealthConfig.MaxSendFailRate < 0 || Config.NetworkHealthConfig.MaxSendFailRate > 1:
		return fmt.Errorf("%s must be in [0,1]", networkHealthMaxSendFailRateKey)
	case Config.NetworkHealthConfig.MaxPortionSendQueueBytesFull < 0 || Config.NetworkHealthConfig.MaxPortionSendQueueBytesFull > 1:
		return fmt.Errorf("%s must be in [0,1]", networkHealthMaxPortionSendQueueFillKey)
	}

	// Network Timeout
	Config.NetworkConfig.InitialTimeout = v.GetDuration(networkInitialTimeoutKey)
	Config.NetworkConfig.MinimumTimeout = v.GetDuration(networkMinimumTimeoutKey)
	Config.NetworkConfig.MaximumTimeout = v.GetDuration(networkMaximumTimeoutKey)
	Config.NetworkConfig.TimeoutHalflife = v.GetDuration(networkTimeoutHalflifeKey)
	Config.NetworkConfig.TimeoutCoefficient = v.GetFloat64(networkTimeoutCoefficientKey)

	switch {
	case Config.NetworkConfig.MinimumTimeout < 1:
		return errors.New("minimum timeout must be positive")
	case Config.NetworkConfig.MinimumTimeout > Config.NetworkConfig.MaximumTimeout:
		return errors.New("maximum timeout can't be less than minimum timeout")
	case Config.NetworkConfig.InitialTimeout < Config.NetworkConfig.MinimumTimeout ||
		Config.NetworkConfig.InitialTimeout > Config.NetworkConfig.MaximumTimeout:
		return errors.New("initial timeout should be in the range [minimumTimeout, maximumTimeout]")
	case Config.NetworkConfig.TimeoutHalflife <= 0:
		return errors.New("network timeout halflife must be positive")
	case Config.NetworkConfig.TimeoutCoefficient < 1:
		return errors.New("network timeout coefficient must be >= 1")
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

	// Load genesis data
	Config.GenesisBytes, Config.AvaxAssetID, err = genesis.Genesis(networkID, v.GetString(genesisConfigFileKey))
	if err != nil {
		return fmt.Errorf("unable to load genesis file: %w", err)
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

	// Indexer
	Config.IndexAllowIncomplete = v.GetBool(indexAllowIncompleteKey)

	// Bootstrap Configs
	Config.RetryBootstrap = v.GetBool(retryBootstrap)
	Config.RetryBootstrapMaxAttempts = v.GetInt(retryBootstrapMaxAttempts)

	// Peer alias
	Config.PeerAliasTimeout = v.GetDuration(peerAliasTimeoutKey)

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
