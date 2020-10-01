// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpcchainvm

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	"github.com/hashicorp/go-plugin"

	"github.com/ava-labs/avalanchego/database/rpcdb"
	"github.com/ava-labs/avalanchego/database/rpcdb/rpcdbproto"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/galiaslookup"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/galiaslookup/galiaslookupproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/ghttpproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gkeystore"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gkeystore/gkeystoreproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/grpcutils"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsharedmemory"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsharedmemory/gsharedmemoryproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsubnetlookup"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsubnetlookup/gsubnetlookupproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/messenger"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/messenger/messengerproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/vmproto"
)

// VMServer is a VM that is managed over RPC.
type VMServer struct {
	vm     block.ChainVM
	broker *plugin.GRPCBroker

	serverCloser grpcutils.ServerCloser
	conns        []*grpc.ClientConn

	ctx      *snow.Context
	toEngine chan common.Message
}

// NewServer returns a vm instance connected to a remote vm instance
func NewServer(vm block.ChainVM, broker *plugin.GRPCBroker) *VMServer {
	return &VMServer{
		vm:     vm,
		broker: broker,
	}
}

// Initialize ...
func (vm *VMServer) Initialize(_ context.Context, req *vmproto.InitializeRequest) (*vmproto.InitializeResponse, error) {
	subnetID, err := ids.ToID(req.SubnetID)
	if err != nil {
		return nil, err
	}
	chainID, err := ids.ToID(req.ChainID)
	if err != nil {
		return nil, err
	}
	nodeID, err := ids.ToShortID(req.NodeID)
	if err != nil {
		return nil, err
	}
	xChainID, err := ids.ToID(req.XChainID)
	if err != nil {
		return nil, err
	}
	avaxAssetID, err := ids.ToID(req.AvaxAssetID)
	if err != nil {
		return nil, err
	}

	dbConn, err := vm.broker.Dial(req.DbServer)
	if err != nil {
		return nil, err
	}
	msgConn, err := vm.broker.Dial(req.EngineServer)
	if err != nil {
		// Ignore DB closing error to return the original error
		_ = dbConn.Close()
		return nil, err
	}
	keystoreConn, err := vm.broker.Dial(req.KeystoreServer)
	if err != nil {
		// Ignore closing error to return the original error
		_ = dbConn.Close()
		_ = msgConn.Close()
		return nil, err
	}
	sharedMemoryConn, err := vm.broker.Dial(req.SharedMemoryServer)
	if err != nil {
		// Ignore closing error to return the original error
		_ = dbConn.Close()
		_ = msgConn.Close()
		_ = keystoreConn.Close()
		return nil, err
	}
	bcLookupConn, err := vm.broker.Dial(req.BcLookupServer)
	if err != nil {
		// Ignore closing error to return the original error
		_ = dbConn.Close()
		_ = msgConn.Close()
		_ = keystoreConn.Close()
		_ = sharedMemoryConn.Close()
		return nil, err
	}
	snLookupConn, err := vm.broker.Dial(req.SnLookupServer)
	if err != nil {
		// Ignore closing error to return the original error
		_ = dbConn.Close()
		_ = msgConn.Close()
		_ = keystoreConn.Close()
		_ = sharedMemoryConn.Close()
		_ = bcLookupConn.Close()
		return nil, err
	}

	dbClient := rpcdb.NewClient(rpcdbproto.NewDatabaseClient(dbConn))
	msgClient := messenger.NewClient(messengerproto.NewMessengerClient(msgConn))
	keystoreClient := gkeystore.NewClient(gkeystoreproto.NewKeystoreClient(keystoreConn), vm.broker)
	sharedMemoryClient := gsharedmemory.NewClient(gsharedmemoryproto.NewSharedMemoryClient(sharedMemoryConn))
	bcLookupClient := galiaslookup.NewClient(galiaslookupproto.NewAliasLookupClient(bcLookupConn))
	snLookupClient := gsubnetlookup.NewClient(gsubnetlookupproto.NewSubnetLookupClient(snLookupConn))

	toEngine := make(chan common.Message, 1)
	go func() {
		for msg := range toEngine {
			// Nothing to do with the error within the goroutine
			_ = msgClient.Notify(msg)
		}
	}()

	vm.ctx = &snow.Context{
		NetworkID:           req.NetworkID,
		SubnetID:            subnetID,
		ChainID:             chainID,
		NodeID:              nodeID,
		XChainID:            xChainID,
		AVAXAssetID:         avaxAssetID,
		Log:                 logging.NoLog{},
		DecisionDispatcher:  nil,
		ConsensusDispatcher: nil,
		Keystore:            keystoreClient,
		SharedMemory:        sharedMemoryClient,
		BCLookup:            bcLookupClient,
		SNLookup:            snLookupClient,
	}

	if err := vm.vm.Initialize(vm.ctx, dbClient, req.GenesisBytes, toEngine, nil); err != nil {
		// Ignore errors closing resources to return the original error
		_ = dbConn.Close()
		_ = msgConn.Close()
		_ = keystoreConn.Close()
		_ = sharedMemoryConn.Close()
		_ = bcLookupConn.Close()
		_ = snLookupConn.Close()
		close(toEngine)
		return nil, err
	}

	vm.conns = append(vm.conns, dbConn)
	vm.conns = append(vm.conns, msgConn)
	vm.toEngine = toEngine
	return &vmproto.InitializeResponse{
		LastAcceptedID: vm.vm.LastAccepted().Bytes(),
	}, nil
}

// Bootstrapping ...
func (vm *VMServer) Bootstrapping(context.Context, *vmproto.BootstrappingRequest) (*vmproto.BootstrappingResponse, error) {
	return &vmproto.BootstrappingResponse{}, vm.vm.Bootstrapping()
}

// Bootstrapped ...
func (vm *VMServer) Bootstrapped(context.Context, *vmproto.BootstrappedRequest) (*vmproto.BootstrappedResponse, error) {
	vm.ctx.Bootstrapped()
	return &vmproto.BootstrappedResponse{}, vm.vm.Bootstrapped()
}

// Shutdown ...
func (vm *VMServer) Shutdown(context.Context, *vmproto.ShutdownRequest) (*vmproto.ShutdownResponse, error) {
	if vm.toEngine == nil {
		return &vmproto.ShutdownResponse{}, nil
	}

	errs := wrappers.Errs{}
	errs.Add(vm.vm.Shutdown())
	close(vm.toEngine)

	vm.serverCloser.Stop()
	for _, conn := range vm.conns {
		errs.Add(conn.Close())
	}
	return &vmproto.ShutdownResponse{}, errs.Err
}

// CreateHandlers ...
func (vm *VMServer) CreateHandlers(_ context.Context, req *vmproto.CreateHandlersRequest) (*vmproto.CreateHandlersResponse, error) {
	handlers := vm.vm.CreateHandlers()
	resp := &vmproto.CreateHandlersResponse{}
	for prefix, h := range handlers {
		handler := h

		// start the messenger server
		serverID := vm.broker.NextId()
		go vm.broker.AcceptAndServe(serverID, func(opts []grpc.ServerOption) *grpc.Server {
			server := grpc.NewServer(opts...)
			vm.serverCloser.Add(server)
			ghttpproto.RegisterHTTPServer(server, ghttp.NewServer(handler.Handler, vm.broker))
			return server
		})

		resp.Handlers = append(resp.Handlers, &vmproto.Handler{
			Prefix:      prefix,
			LockOptions: uint32(handler.LockOptions),
			Server:      serverID,
		})
	}
	return resp, nil
}

// BuildBlock ...
func (vm *VMServer) BuildBlock(_ context.Context, _ *vmproto.BuildBlockRequest) (*vmproto.BuildBlockResponse, error) {
	blk, err := vm.vm.BuildBlock()
	if err != nil {
		return nil, err
	}
	return &vmproto.BuildBlockResponse{
		Id:       blk.ID().Bytes(),
		ParentID: blk.Parent().Bytes(),
		Bytes:    blk.Bytes(),
	}, nil
}

// ParseBlock ...
func (vm *VMServer) ParseBlock(_ context.Context, req *vmproto.ParseBlockRequest) (*vmproto.ParseBlockResponse, error) {
	blk, err := vm.vm.ParseBlock(req.Bytes)
	if err != nil {
		return nil, err
	}
	return &vmproto.ParseBlockResponse{
		Id:       blk.ID().Bytes(),
		ParentID: blk.Parent().Bytes(),
		Status:   uint32(blk.Status()),
	}, nil
}

// SaveBlock persists a block to the database.
// [req.Bytes] is the byte representation of the block to save.
func (vm *VMServer) SaveBlock(_ context.Context, req *vmproto.SaveBlockRequest) (*vmproto.SaveBlockResponse, error) {
	blk, err := vm.vm.ParseBlock(req.Bytes)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse block: %w", err)
	}
	return &vmproto.SaveBlockResponse{}, vm.vm.SaveBlock(blk)
}

// GetBlock ...
func (vm *VMServer) GetBlock(_ context.Context, req *vmproto.GetBlockRequest) (*vmproto.GetBlockResponse, error) {
	id, err := ids.ToID(req.Id)
	if err != nil {
		return nil, err
	}
	blk, err := vm.vm.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return &vmproto.GetBlockResponse{
		ParentID: blk.Parent().Bytes(),
		Bytes:    blk.Bytes(),
		Status:   uint32(blk.Status()),
	}, nil
}

// SetPreference ...
func (vm *VMServer) SetPreference(_ context.Context, req *vmproto.SetPreferenceRequest) (*vmproto.SetPreferenceResponse, error) {
	id, err := ids.ToID(req.Id)
	if err != nil {
		return nil, err
	}
	vm.vm.SetPreference(id)
	return &vmproto.SetPreferenceResponse{}, nil
}

// BlockVerify ...
func (vm *VMServer) BlockVerify(_ context.Context, req *vmproto.BlockVerifyRequest) (*vmproto.BlockVerifyResponse, error) {
	id, err := ids.ToID(req.Id)
	if err != nil {
		return nil, err
	}
	blk, err := vm.vm.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return &vmproto.BlockVerifyResponse{}, blk.Verify()
}

// BlockAccept ...
func (vm *VMServer) BlockAccept(_ context.Context, req *vmproto.BlockAcceptRequest) (*vmproto.BlockAcceptResponse, error) {
	id, err := ids.ToID(req.Id)
	if err != nil {
		return nil, err
	}
	blk, err := vm.vm.GetBlock(id)
	if err != nil {
		return nil, err
	}
	if err := blk.Accept(); err != nil {
		return nil, err
	}
	return &vmproto.BlockAcceptResponse{}, nil
}

// BlockReject ...
func (vm *VMServer) BlockReject(_ context.Context, req *vmproto.BlockRejectRequest) (*vmproto.BlockRejectResponse, error) {
	id, err := ids.ToID(req.Id)
	if err != nil {
		return nil, err
	}
	blk, err := vm.vm.GetBlock(id)
	if err != nil {
		return nil, err
	}
	if err := blk.Reject(); err != nil {
		return nil, err
	}
	return &vmproto.BlockRejectResponse{}, nil
}
