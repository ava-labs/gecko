// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package admin

import (
	"errors"
	"net/http"

	"github.com/gorilla/rpc/v2"

	"github.com/ava-labs/avalanchego/api"
	"github.com/ava-labs/avalanchego/api/server"
	"github.com/ava-labs/avalanchego/chains"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/perms"

	cjson "github.com/ava-labs/avalanchego/utils/json"
)

const (
	maxAliasLength = 512

	// Name of file that stacktraces are written to
	stacktraceFile = "stacktrace.txt"
)

var (
	errAliasTooLong = errors.New("alias length is too long")
)

// Admin is the API service for node admin management
type Admin struct {
	log          logging.Logger
	performance  *Performance
	chainManager chains.Manager
	httpServer   *server.Server
}

// NewService returns a new admin API service
func NewService(log logging.Logger, chainManager chains.Manager, httpServer *server.Server) (*common.HTTPHandler, error) {
	newServer := rpc.NewServer()
	codec := cjson.NewCodec()
	newServer.RegisterCodec(codec, "application/json")
	newServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	if err := newServer.RegisterService(&Admin{
		log:          log,
		chainManager: chainManager,
		httpServer:   httpServer,
		performance:  NewDefaultPerformanceService(),
	}, "admin"); err != nil {
		return nil, err
	}
	return &common.HTTPHandler{Handler: newServer}, nil
}

// StartCPUProfiler starts a cpu profile writing to the specified file
func (service *Admin) StartCPUProfiler(_ *http.Request, _ *struct{}, reply *api.SuccessResponse) error {
	service.log.Info("Admin: StartCPUProfiler called")
	reply.Success = true
	return service.performance.StartCPUProfiler()
}

// StopCPUProfiler stops the cpu profile
func (service *Admin) StopCPUProfiler(_ *http.Request, _ *struct{}, reply *api.SuccessResponse) error {
	service.log.Info("Admin: StopCPUProfiler called")

	reply.Success = true
	return service.performance.StopCPUProfiler()
}

// MemoryProfile runs a memory profile writing to the specified file
func (service *Admin) MemoryProfile(_ *http.Request, _ *struct{}, reply *api.SuccessResponse) error {
	service.log.Info("Admin: MemoryProfile called")

	reply.Success = true
	return service.performance.MemoryProfile()
}

// LockProfile runs a mutex profile writing to the specified file
func (service *Admin) LockProfile(_ *http.Request, _ *struct{}, reply *api.SuccessResponse) error {
	service.log.Info("Admin: LockProfile called")

	reply.Success = true
	return service.performance.LockProfile()
}

// AliasArgs are the arguments for calling Alias
type AliasArgs struct {
	Endpoint string `json:"endpoint"`
	Alias    string `json:"alias"`
}

// Alias attempts to alias an HTTP endpoint to a new name
func (service *Admin) Alias(_ *http.Request, args *AliasArgs, reply *api.SuccessResponse) error {
	service.log.Info("Admin: Alias called with URL: %s, Alias: %s", args.Endpoint, args.Alias)

	if len(args.Alias) > maxAliasLength {
		return errAliasTooLong
	}

	reply.Success = true
	return service.httpServer.AddAliasesWithReadLock(args.Endpoint, args.Alias)
}

// AliasChainArgs are the arguments for calling AliasChain
type AliasChainArgs struct {
	Chain string `json:"chain"`
	Alias string `json:"alias"`
}

// AliasChain attempts to alias a chain to a new name
func (service *Admin) AliasChain(_ *http.Request, args *AliasChainArgs, reply *api.SuccessResponse) error {
	service.log.Info("Admin: AliasChain called with Chain: %s, Alias: %s", args.Chain, args.Alias)

	if len(args.Alias) > maxAliasLength {
		return errAliasTooLong
	}
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

// GetChainAliasesArgs are the arguments for calling GetChainAliases
type GetChainAliasesArgs struct {
	Chain string `json:"chain"`
}

// GetChainAliasesReply are the aliases of the given chain
type GetChainAliasesReply struct {
	Aliases []string `json:"aliases"`
}

// GetChainAliases returns the aliases of the chain
func (service *Admin) GetChainAliases(r *http.Request, args *GetChainAliasesArgs, reply *GetChainAliasesReply) error {
	service.log.Info("Admin: GetChainAliases called with Chain: %s", args.Chain)

	id, err := ids.FromString(args.Chain)
	if err != nil {
		return err
	}

	reply.Aliases = service.chainManager.Aliases(id)
	return nil
}

// Stacktrace returns the current global stacktrace
func (service *Admin) Stacktrace(_ *http.Request, _ *struct{}, reply *api.SuccessResponse) error {
	service.log.Info("Admin: Stacktrace called")

	reply.Success = true
	stacktrace := []byte(logging.Stacktrace{Global: true}.String())
	return perms.WriteFile(stacktraceFile, stacktrace, perms.ReadWrite)
}
