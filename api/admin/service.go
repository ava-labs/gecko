// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package admin

import (
	"net/http"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/gecko/api"
	"github.com/ava-labs/gecko/chains"
	"github.com/ava-labs/gecko/genesis"
	"github.com/ava-labs/gecko/ids"
	"github.com/ava-labs/gecko/network"
	"github.com/ava-labs/gecko/snow/engine/common"
	"github.com/ava-labs/gecko/utils/logging"
	"github.com/ava-labs/gecko/version"

	cjson "github.com/ava-labs/gecko/utils/json"
)

// Admin is the API service for node admin management
type Admin struct {
	version      version.Version
	nodeID       ids.ShortID
	networkID    uint32
	log          logging.Logger
	networking   network.Network
	performance  Performance
	chainManager chains.Manager
	httpServer   *api.Server
}

var mapping map[string]string = map[string]string{
	"/node/id":        "GetNodeID",
	"/blockchain/id":  "GetBlockchainID",
	"/network/id":     "GetNetworkID",
	"/peers":          "Peers",
	"/profile/start":  "StartCPUProfiler",
	"/profile/stop":   "StopCPUProfiler",
	"/profile/memory": "MemoryProfile",
	"/profile/lock":   "LockProfile",
	"/stack/trace":    "Stacktrace",
	"/chain/alias":    "AliasChain",
}

// NewService returns a new admin API service
func NewService(version version.Version, nodeID ids.ShortID, networkID uint32, log logging.Logger, chainManager chains.Manager, peers network.Network, httpServer *api.Server) *common.HTTPHandler {
	newServer := rpc.NewServer()
	restMap := cjson.MappingGenerator(mapping, "admin")
	codec := cjson.RestCodec{Mapping: restMap}
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	newServer.RegisterService(&Admin{
		version:      version,
		nodeID:       nodeID,
		networkID:    networkID,
		log:          log,
		chainManager: chainManager,
		networking:   peers,
		httpServer:   httpServer,
	}, "admin")
	return &common.HTTPHandler{Handler: newServer, RestEndpoints: restMap.GetKeys()}
}

// GetNodeVersionReply are the results from calling GetNodeVersion
type GetNodeVersionReply struct {
	Version string `json:"version"`
}

// GetNodeVersion returns the version this node is running
func (service *Admin) GetNodeVersion(_ *http.Request, _ *struct{}, reply *GetNodeVersionReply) error {
	service.log.Info("Admin: GetNodeVersion called")

	reply.Version = service.version.String()
	return nil
}

// GetNodeIDReply are the results from calling GetNodeID
type GetNodeIDReply struct {
	NodeID ids.ShortID `json:"nodeID"`
}

// GetNodeID returns the node ID of this node
func (service *Admin) GetNodeID(_ *http.Request, _ *struct{}, reply *GetNodeIDReply) error {
	service.log.Info("Admin: GetNodeID called")

	reply.NodeID = service.nodeID
	return nil
}

// GetNetworkIDReply are the results from calling GetNetworkID
type GetNetworkIDReply struct {
	NetworkID cjson.Uint32 `json:"networkID"`
}

// GetNetworkID returns the network ID this node is running on
func (service *Admin) GetNetworkID(_ *http.Request, _ *struct{}, reply *GetNetworkIDReply) error {
	service.log.Info("Admin: GetNetworkID called")

	reply.NetworkID = cjson.Uint32(service.networkID)
	return nil
}

// GetNetworkNameReply is the result from calling GetNetworkName
type GetNetworkNameReply struct {
	NetworkName string `json:"networkName"`
}

// GetNetworkName returns the network name this node is running on
func (service *Admin) GetNetworkName(_ *http.Request, _ *struct{}, reply *GetNetworkNameReply) error {
	service.log.Info("Admin: GetNetworkName called")

	reply.NetworkName = genesis.NetworkName(service.networkID)
	return nil
}

// GetBlockchainIDArgs are the arguments for calling GetBlockchainID
type GetBlockchainIDArgs struct {
	Alias string `json:"alias"`
}

// GetBlockchainIDReply are the results from calling GetBlockchainID
type GetBlockchainIDReply struct {
	BlockchainID string `json:"blockchainID"`
}

// GetBlockchainID returns the blockchain ID that resolves the alias that was supplied
func (service *Admin) GetBlockchainID(_ *http.Request, args *GetBlockchainIDArgs, reply *GetBlockchainIDReply) error {
	service.log.Info("Admin: GetBlockchainID called")

	bID, err := service.chainManager.Lookup(args.Alias)
	reply.BlockchainID = bID.String()
	return err
}

// PeersReply are the results from calling Peers
type PeersReply struct {
	Peers []network.PeerID `json:"peers"`
}

// Peers returns the list of current validators
func (service *Admin) Peers(_ *http.Request, _ *struct{}, reply *PeersReply) error {
	service.log.Info("Admin: Peers called")
	reply.Peers = service.networking.Peers()
	return nil
}

// StartCPUProfilerReply are the results from calling StartCPUProfiler
type StartCPUProfilerReply struct {
	Success bool `json:"success"`
}

// StartCPUProfiler starts a cpu profile writing to the specified file
func (service *Admin) StartCPUProfiler(_ *http.Request, args *struct{}, reply *StartCPUProfilerReply) error {
	service.log.Info("Admin: StartCPUProfiler called")
	reply.Success = true
	return service.performance.StartCPUProfiler()
}

// StopCPUProfilerReply are the results from calling StopCPUProfiler
type StopCPUProfilerReply struct {
	Success bool `json:"success"`
}

// StopCPUProfiler stops the cpu profile
func (service *Admin) StopCPUProfiler(_ *http.Request, _ *struct{}, reply *StopCPUProfilerReply) error {
	service.log.Info("Admin: StopCPUProfiler called")
	reply.Success = true
	return service.performance.StopCPUProfiler()
}

// MemoryProfileReply are the results from calling MemoryProfile
type MemoryProfileReply struct {
	Success bool `json:"success"`
}

// MemoryProfile runs a memory profile writing to the specified file
func (service *Admin) MemoryProfile(_ *http.Request, args *struct{}, reply *MemoryProfileReply) error {
	service.log.Info("Admin: MemoryProfile called")
	reply.Success = true
	return service.performance.MemoryProfile()
}

// LockProfileReply are the results from calling LockProfile
type LockProfileReply struct {
	Success bool `json:"success"`
}

// LockProfile runs a mutex profile writing to the specified file
func (service *Admin) LockProfile(_ *http.Request, args *struct{}, reply *LockProfileReply) error {
	service.log.Info("Admin: LockProfile called")
	reply.Success = true
	return service.performance.LockProfile()
}

// AliasArgs are the arguments for calling Alias
type AliasArgs struct {
	Endpoint string `json:"endpoint"`
	Alias    string `json:"alias"`
}

// AliasReply are the results from calling Alias
type AliasReply struct {
	Success bool `json:"success"`
}

// Alias attempts to alias an HTTP endpoint to a new name
func (service *Admin) Alias(_ *http.Request, args *AliasArgs, reply *AliasReply) error {
	service.log.Info("Admin: Alias called with URL: %s, Alias: %s", args.Endpoint, args.Alias)
	reply.Success = true
	return service.httpServer.AddAliasesWithReadLock(args.Endpoint, args.Alias)
}

// AliasChainArgs are the arguments for calling AliasChain
type AliasChainArgs struct {
	Chain string `json:"chain"`
	Alias string `json:"alias"`
}

// AliasChainReply are the results from calling AliasChain
type AliasChainReply struct {
	Success bool `json:"success"`
}

// AliasChain attempts to alias a chain to a new name
func (service *Admin) AliasChain(_ *http.Request, args *AliasChainArgs, reply *AliasChainReply) error {
	service.log.Info("Admin: AliasChain called with Chain: %s, Alias: %s", args.Chain, args.Alias)

	chainID, err := service.chainManager.Lookup(args.Chain)
	if err != nil {
		return err
	}

	if err := service.chainManager.Alias(chainID, args.Alias); err != nil {
		return err
	}

	reply.Success = true
	return service.httpServer.AddAliasesWithReadLock("bc/"+chainID.String(), "bc/"+args.Alias)
}

// StacktraceArgs are the arguments for calling Stacktrace
type StacktraceArgs struct{}

// StacktraceReply are the results from calling Stacktrace
type StacktraceReply struct {
	Stacktrace string `json:"stacktrace"`
}

// Stacktrace returns the current global stacktrace
func (service *Admin) Stacktrace(_ *http.Request, _ *struct{}, reply *StacktraceReply) error {
	reply.Stacktrace = logging.Stacktrace{Global: true}.String()
	return nil
}
