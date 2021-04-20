// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import (
	"errors"
	"testing"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/snow"
)

var (
	errInitialize           = errors.New("unexpectedly called Initialize")
	errBootstrapping        = errors.New("unexpectedly called Bootstrapping")
	errBootstrapped         = errors.New("unexpectedly called Bootstrapped")
	errShutdown             = errors.New("unexpectedly called Shutdown")
	errCreateHandlers       = errors.New("unexpectedly called CreateHandlers")
	errCreateStaticHandlers = errors.New("unexpectedly called CreateStaticHandlers")
	errHealthCheck          = errors.New("unexpectedly called HealthCheck")

	_ VM = &TestVM{}
)

// TestVM is a test vm
type TestVM struct {
	T *testing.T

	CantInitialize, CantBootstrapping, CantBootstrapped,
	CantShutdown, CantCreateHandlers, CantCreateStaticHandlers,
	CantHealthCheck bool

	InitializeF                              func(*snow.Context, database.Database, []byte, chan<- Message, []*Fx) error
	BootstrappingF, BootstrappedF, ShutdownF func() error
	CreateHandlersF                          func() (map[string]*HTTPHandler, error)
	CreateStaticHandlersF                    func() (map[string]*HTTPHandler, error)
	HealthCheckF                             func() (interface{}, error)
}

func (vm *TestVM) Default(cant bool) {
	vm.CantInitialize = cant
	vm.CantBootstrapping = cant
	vm.CantBootstrapped = cant
	vm.CantShutdown = cant
	vm.CantCreateHandlers = cant
	vm.CantCreateStaticHandlers = cant
	vm.CantHealthCheck = cant
}

func (vm *TestVM) Initialize(ctx *snow.Context, db database.Database, initState []byte, msgChan chan<- Message, fxs []*Fx) error {
	if vm.InitializeF != nil {
		return vm.InitializeF(ctx, db, initState, msgChan, fxs)
	}
	if vm.CantInitialize && vm.T != nil {
		vm.T.Fatal(errInitialize)
	}
	return errInitialize
}

func (vm *TestVM) Bootstrapping() error {
	if vm.BootstrappingF != nil {
		return vm.BootstrappingF()
	}
	if vm.CantBootstrapping {
		if vm.T != nil {
			vm.T.Fatal(errBootstrapping)
		}
		return errBootstrapping
	}
	return nil
}

func (vm *TestVM) Bootstrapped() error {
	if vm.BootstrappedF != nil {
		return vm.BootstrappedF()
	}
	if vm.CantBootstrapped {
		if vm.T != nil {
			vm.T.Fatal(errBootstrapped)
		}
		return errBootstrapped
	}
	return nil
}

func (vm *TestVM) Shutdown() error {
	if vm.ShutdownF != nil {
		return vm.ShutdownF()
	}
	if vm.CantShutdown {
		if vm.T != nil {
			vm.T.Fatal(errShutdown)
		}
		return errShutdown
	}
	return nil
}

func (vm *TestVM) CreateHandlers() (map[string]*HTTPHandler, error) {
	if vm.CreateHandlersF != nil {
		return vm.CreateHandlersF()
	}
	if vm.CantCreateHandlers && vm.T != nil {
		vm.T.Fatal(errCreateHandlers)
	}
	return nil, nil
}

func (vm *TestVM) CreateStaticHandlers() (map[string]*HTTPHandler, error) {
	if vm.CreateStaticHandlersF != nil {
		return vm.CreateStaticHandlersF()
	}
	if vm.CantCreateStaticHandlers && vm.T != nil {
		vm.T.Fatal(errCreateStaticHandlers)
	}
	return nil, nil
}

func (vm *TestVM) HealthCheck() (interface{}, error) {
	if vm.HealthCheckF != nil {
		return vm.HealthCheckF()
	}
	if vm.CantHealthCheck && vm.T != nil {
		vm.T.Fatal(errHealthCheck)
	}
	return nil, errHealthCheck
}
