package rpcapi

import (
	"github.com/ava-labs/avalanchego/snow/engine/common"
	cjson "github.com/ava-labs/avalanchego/utils/json"
	"github.com/ava-labs/avalanchego/vms/avm/internalavm"
	v1 "github.com/ava-labs/avalanchego/vms/avm/rpcapi/v1"
	v2 "github.com/ava-labs/avalanchego/vms/avm/rpcapi/v2"
	"github.com/ava-labs/avalanchego/vms/avm/service"
	"github.com/gorilla/rpc/v2"
)

type RPCAPI struct {
}

func NewRPCAPI() *RPCAPI {
	return &RPCAPI{}
}

// CreateHandlers implements the avalanche.DAGVM interface
func (api *RPCAPI) CreateHandlers(vmIntf interface{}, service *service.Service) map[string]*common.HTTPHandler {
	//vm.metrics.numCreateHandlersCalls.Inc()

	vm, ok := vmIntf.(*internalavm.VM)
	if !ok {
		return nil
	}

	codec := cjson.NewCodec()

	v1RPCServer := rpc.NewServer()
	v1RPCServer.RegisterCodec(codec, "application/json")
	v1RPCServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	// name this service "avm"
	vm.Context().Log.AssertNoError(v1RPCServer.RegisterService(v1.NewController(vm), "avm"))

	//walletServer := rpc.NewServer()
	//walletServer.RegisterCodec(codec, "application/json")
	//walletServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	//// name this service "avm"
	//vm.Context().Log.AssertNoError(walletServer.RegisterService(&vm.walletService, "wallet"))

	v2RPCServer := rpc.NewServer()
	v2RPCServer.RegisterCodec(codec, "application/json")
	v2RPCServer.RegisterCodec(codec, "application/json;charset=UTF-8")

	// register v1 services for "avm"
	vm.Context().Log.AssertNoError(v2RPCServer.RegisterService(v2.NewController(service), "avm"))

	return map[string]*common.HTTPHandler{
		"": {Handler: v1RPCServer},
		//"/wallet": {Handler: walletServer},
		"/v2": {Handler: v2RPCServer},
		//"/pubsub": {LockOptions: common.NoLock, Handler: vm.pubsub},
	}
}

// CreateHandlers implements the avalanche.DAGVM interface
func (api *RPCAPI) CreateStaticHandlers(vmIntf interface{}) map[string]*common.HTTPHandler {
	vm, ok := vmIntf.(*internalavm.VM)
	if !ok {
		return nil
	}

	codec := cjson.NewCodec()

	v1RPCServer := rpc.NewServer()
	v1RPCServer.RegisterCodec(codec, "application/json")
	v1RPCServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	// name this service "avm"
	vm.Context().Log.AssertNoError(v1RPCServer.RegisterService(v1.NewStaticController(), "avm"))

	//walletServer := rpc.NewServer()
	//walletServer.RegisterCodec(codec, "application/json")
	//walletServer.RegisterCodec(codec, "application/json;charset=UTF-8")
	//// name this service "avm"
	//vm.Context().Log.AssertNoError(walletServer.RegisterService(&vm.walletService, "wallet"))

	v2RPCServer := rpc.NewServer()
	v2RPCServer.RegisterCodec(codec, "application/json")
	v2RPCServer.RegisterCodec(codec, "application/json;charset=UTF-8")

	// register v1 services for "avm"
	vm.Context().Log.AssertNoError(v2RPCServer.RegisterService(v2.NewStaticController(), "avm"))

	return map[string]*common.HTTPHandler{
		"":    {LockOptions: common.WriteLock, Handler: v1RPCServer},
		"/v2": {LockOptions: common.WriteLock, Handler: v2RPCServer},
	}
}
