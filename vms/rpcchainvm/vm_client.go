// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package rpcchainvm

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"

	"github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/api/keystore/gkeystore"
	"github.com/ava-labs/avalanchego/api/keystore/gkeystore/gkeystoreproto"
	"github.com/ava-labs/avalanchego/cache"
	"github.com/ava-labs/avalanchego/cache/metercacher"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/rpcdb"
	"github.com/ava-labs/avalanchego/database/rpcdb/rpcdbproto"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/utils/wrappers"
	"github.com/ava-labs/avalanchego/vms/components/missing"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/galiaslookup"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/galiaslookup/galiaslookupproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/ghttp/ghttpproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/grpcutils"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsharedmemory"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsharedmemory/gsharedmemoryproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsubnetlookup"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/gsubnetlookup/gsubnetlookupproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/messenger"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/messenger/messengerproto"
	"github.com/ava-labs/avalanchego/vms/rpcchainvm/vmproto"
)

var (
	errUnsupportedFXs = errors.New("unsupported feature extensions")

	_ block.ChainVM = &VMClient{}
)

const (
	decidedCacheSize = 500
)

// VMClient is an implementation of VM that talks over RPC.
type VMClient struct {
	client vmproto.VMClient
	broker *plugin.GRPCBroker
	proc   *plugin.Client

	db           *rpcdb.DatabaseServer
	messenger    *messenger.Server
	keystore     *gkeystore.Server
	sharedMemory *gsharedmemory.Server
	bcLookup     *galiaslookup.Server
	snLookup     *gsubnetlookup.Server

	serverCloser grpcutils.ServerCloser
	conns        []*grpc.ClientConn

	ctx  *snow.Context
	blks map[ids.ID]*BlockClient

	decidedBlocks cache.Cacher

	lastAccepted ids.ID
}

// NewClient returns a VM connected to a remote VM
func NewClient(client vmproto.VMClient, broker *plugin.GRPCBroker) *VMClient {
	return &VMClient{
		client: client,
		broker: broker,
		blks:   make(map[ids.ID]*BlockClient),
	}
}

// SetProcess gives ownership of the server process to the client.
func (vm *VMClient) SetProcess(proc *plugin.Client) {
	vm.proc = proc
}

// initializeCaches creates a new [decidedBlocks] and [missingBlocks] cache. It
// wraps these caches in metercacher so that we can get prometheus metrics
// about their performance.
func (vm *VMClient) initializeCaches(registerer prometheus.Registerer, namespace string) error {
	decidedCache, err := metercacher.New(
		fmt.Sprintf("%s_rpcchainvm_decided_cache", namespace),
		registerer,
		&cache.LRU{Size: decidedCacheSize},
	)
	if err != nil {
		return fmt.Errorf("could not initialize decided blocks cache: %w", err)
	}
	vm.decidedBlocks = decidedCache

	return nil
}

func (vm *VMClient) Initialize(
	ctx *snow.Context,
	db database.Database,
	genesisBytes []byte,
	toEngine chan<- common.Message,
	fxs []*common.Fx,
) error {
	if len(fxs) != 0 {
		return errUnsupportedFXs
	}

	epochFirstTransitionBytes, err := ctx.EpochFirstTransition.MarshalBinary()
	if err != nil {
		return err
	}

	vm.ctx = ctx

	vm.db = rpcdb.NewServer(db)
	vm.messenger = messenger.NewServer(toEngine)
	vm.keystore = gkeystore.NewServer(ctx.Keystore, vm.broker)
	vm.sharedMemory = gsharedmemory.NewServer(ctx.SharedMemory, db)
	vm.bcLookup = galiaslookup.NewServer(ctx.BCLookup)
	vm.snLookup = gsubnetlookup.NewServer(ctx.SNLookup)

	if err := vm.initializeCaches(ctx.Metrics, ctx.Namespace); err != nil {
		return err
	}

	// start the db server
	dbBrokerID := vm.broker.NextId()
	go vm.broker.AcceptAndServe(dbBrokerID, vm.startDBServer)

	// start the messenger server
	messengerBrokerID := vm.broker.NextId()
	go vm.broker.AcceptAndServe(messengerBrokerID, vm.startMessengerServer)

	// start the keystore server
	keystoreBrokerID := vm.broker.NextId()
	go vm.broker.AcceptAndServe(keystoreBrokerID, vm.startKeystoreServer)

	// start the shared memory server
	sharedMemoryBrokerID := vm.broker.NextId()
	go vm.broker.AcceptAndServe(sharedMemoryBrokerID, vm.startSharedMemoryServer)

	// start the blockchain alias server
	bcLookupBrokerID := vm.broker.NextId()
	go vm.broker.AcceptAndServe(bcLookupBrokerID, vm.startBCLookupServer)

	// start the subnet alias server
	snLookupBrokerID := vm.broker.NextId()
	go vm.broker.AcceptAndServe(snLookupBrokerID, vm.startSNLookupServer)

	resp, err := vm.client.Initialize(context.Background(), &vmproto.InitializeRequest{
		NetworkID:            ctx.NetworkID,
		SubnetID:             ctx.SubnetID[:],
		ChainID:              ctx.ChainID[:],
		NodeID:               ctx.NodeID.Bytes(),
		XChainID:             ctx.XChainID[:],
		AvaxAssetID:          ctx.AVAXAssetID[:],
		GenesisBytes:         genesisBytes,
		DbServer:             dbBrokerID,
		EngineServer:         messengerBrokerID,
		KeystoreServer:       keystoreBrokerID,
		SharedMemoryServer:   sharedMemoryBrokerID,
		BcLookupServer:       bcLookupBrokerID,
		SnLookupServer:       snLookupBrokerID,
		EpochFirstTransition: epochFirstTransitionBytes,
		EpochDuration:        uint64(ctx.EpochDuration),
	})
	if err != nil {
		return err
	}

	lastAccepted, err := ids.ToID(resp.LastAcceptedID)
	if err != nil {
		return err
	}

	vm.lastAccepted = lastAccepted
	return nil
}

func (vm *VMClient) startDBServer(opts []grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	vm.serverCloser.Add(server)
	rpcdbproto.RegisterDatabaseServer(server, vm.db)
	return server
}

func (vm *VMClient) startMessengerServer(opts []grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	vm.serverCloser.Add(server)
	messengerproto.RegisterMessengerServer(server, vm.messenger)
	return server
}

func (vm *VMClient) startKeystoreServer(opts []grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	vm.serverCloser.Add(server)
	gkeystoreproto.RegisterKeystoreServer(server, vm.keystore)
	return server
}

func (vm *VMClient) startSharedMemoryServer(opts []grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	vm.serverCloser.Add(server)
	gsharedmemoryproto.RegisterSharedMemoryServer(server, vm.sharedMemory)
	return server
}

func (vm *VMClient) startBCLookupServer(opts []grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	vm.serverCloser.Add(server)
	galiaslookupproto.RegisterAliasLookupServer(server, vm.bcLookup)
	return server
}

func (vm *VMClient) startSNLookupServer(opts []grpc.ServerOption) *grpc.Server {
	server := grpc.NewServer(opts...)
	vm.serverCloser.Add(server)
	gsubnetlookupproto.RegisterSubnetLookupServer(server, vm.snLookup)
	return server
}

func (vm *VMClient) Bootstrapping() error {
	_, err := vm.client.Bootstrapping(context.Background(), &vmproto.BootstrappingRequest{})
	return err
}

func (vm *VMClient) Bootstrapped() error {
	_, err := vm.client.Bootstrapped(context.Background(), &vmproto.BootstrappedRequest{})
	return err
}

func (vm *VMClient) Shutdown() error {
	errs := wrappers.Errs{}
	_, err := vm.client.Shutdown(context.Background(), &vmproto.ShutdownRequest{})
	errs.Add(err)

	vm.serverCloser.Stop()
	for _, conn := range vm.conns {
		errs.Add(conn.Close())
	}

	vm.proc.Kill()
	return errs.Err
}

func (vm *VMClient) CreateHandlers() (map[string]*common.HTTPHandler, error) {
	resp, err := vm.client.CreateHandlers(context.Background(), &vmproto.CreateHandlersRequest{})
	if err != nil {
		return nil, err
	}

	handlers := make(map[string]*common.HTTPHandler, len(resp.Handlers))
	for _, handler := range resp.Handlers {
		conn, err := vm.broker.Dial(handler.Server)
		if err != nil {
			return nil, err
		}

		vm.conns = append(vm.conns, conn)
		handlers[handler.Prefix] = &common.HTTPHandler{
			LockOptions: common.LockOption(handler.LockOptions),
			Handler:     ghttp.NewClient(ghttpproto.NewHTTPClient(conn), vm.broker),
		}
	}
	return handlers, nil
}

func (vm *VMClient) BuildBlock() (snowman.Block, error) {
	resp, err := vm.client.BuildBlock(context.Background(), &vmproto.BuildBlockRequest{})
	if err != nil {
		return nil, err
	}

	id, err := ids.ToID(resp.Id)
	vm.ctx.Log.AssertNoError(err)

	parentID, err := ids.ToID(resp.ParentID)
	vm.ctx.Log.AssertNoError(err)

	return &BlockClient{
		vm:       vm,
		id:       id,
		parentID: parentID,
		status:   choices.Processing,
		bytes:    resp.Bytes,
		height:   resp.Height,
	}, nil
}

func (vm *VMClient) ParseBlock(bytes []byte) (snowman.Block, error) {
	resp, err := vm.client.ParseBlock(context.Background(), &vmproto.ParseBlockRequest{
		Bytes: bytes,
	})
	if err != nil {
		return nil, err
	}

	id, err := ids.ToID(resp.Id)
	vm.ctx.Log.AssertNoError(err)

	if blk, cached := vm.blks[id]; cached {
		return blk, nil
	}
	if blkIntf, cached := vm.decidedBlocks.Get(id); cached {
		return blkIntf.(*BlockClient), nil
	}

	parentID, err := ids.ToID(resp.ParentID)
	vm.ctx.Log.AssertNoError(err)

	status := choices.Status(resp.Status)
	vm.ctx.Log.AssertDeferredNoError(status.Valid)

	blk := &BlockClient{
		vm:       vm,
		id:       id,
		parentID: parentID,
		status:   status,
		bytes:    bytes,
		height:   resp.Height,
	}

	if status.Decided() {
		vm.decidedBlocks.Put(id, blk)
	}
	return blk, nil
}

func (vm *VMClient) GetBlock(id ids.ID) (snowman.Block, error) {
	if blk, cached := vm.blks[id]; cached {
		return blk, nil
	}
	if blkIntf, cached := vm.decidedBlocks.Get(id); cached {
		return blkIntf.(*BlockClient), nil
	}

	resp, err := vm.client.GetBlock(context.Background(), &vmproto.GetBlockRequest{
		Id: id[:],
	})
	if err != nil {
		return nil, err
	}

	parentID, err := ids.ToID(resp.ParentID)
	vm.ctx.Log.AssertNoError(err)
	status := choices.Status(resp.Status)
	vm.ctx.Log.AssertDeferredNoError(status.Valid)

	blk := &BlockClient{
		vm:       vm,
		id:       id,
		parentID: parentID,
		status:   status,
		bytes:    resp.Bytes,
		height:   resp.Height,
	}

	if status.Decided() {
		vm.decidedBlocks.Put(id, blk)
	}
	return blk, nil
}

func (vm *VMClient) SetPreference(id ids.ID) error {
	_, err := vm.client.SetPreference(context.Background(), &vmproto.SetPreferenceRequest{
		Id: id[:],
	})
	return err
}

func (vm *VMClient) LastAccepted() (ids.ID, error) { return vm.lastAccepted, nil }

func (vm *VMClient) HealthCheck() (interface{}, error) {
	return vm.client.Health(
		context.Background(),
		&vmproto.HealthRequest{},
	)
}

// BlockClient is an implementation of Block that talks over RPC.
type BlockClient struct {
	vm *VMClient

	id       ids.ID
	parentID ids.ID
	status   choices.Status
	bytes    []byte
	height   uint64
}

func (b *BlockClient) ID() ids.ID { return b.id }

func (b *BlockClient) Accept() error {
	delete(b.vm.blks, b.id)
	b.status = choices.Accepted
	_, err := b.vm.client.BlockAccept(context.Background(), &vmproto.BlockAcceptRequest{
		Id: b.id[:],
	})
	if err != nil {
		return err
	}

	b.vm.decidedBlocks.Put(b.id, b)
	b.vm.lastAccepted = b.id
	return nil
}

func (b *BlockClient) Reject() error {
	delete(b.vm.blks, b.id)
	b.status = choices.Rejected
	_, err := b.vm.client.BlockReject(context.Background(), &vmproto.BlockRejectRequest{
		Id: b.id[:],
	})

	b.vm.decidedBlocks.Put(b.id, b)
	return err
}

func (b *BlockClient) Status() choices.Status { return b.status }

func (b *BlockClient) Parent() snowman.Block {
	if parent, err := b.vm.GetBlock(b.parentID); err == nil {
		return parent
	}
	return &missing.Block{BlkID: b.parentID}
}

func (b *BlockClient) Verify() error {
	_, err := b.vm.client.BlockVerify(context.Background(), &vmproto.BlockVerifyRequest{
		Id: b.id[:],
	})
	if err != nil {
		return err
	}

	b.vm.blks[b.id] = b
	return nil
}

func (b *BlockClient) Bytes() []byte  { return b.bytes }
func (b *BlockClient) Height() uint64 { return b.height }
