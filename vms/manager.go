// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vms

import (
	"fmt"
	"sync"

	"github.com/ava-labs/avalanchego/api/server"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/logging"
)

// A VMFactory creates new instances of a VM
type VMFactory interface {
	New(*snow.Context) (interface{}, error)
}

// Manager is a VM manager.
// It has the following functionality:
//   1) Register a VM factory. To register a VM is to associate its ID with a
//		 VMFactory which, when New() is called upon it, creates a new instance of that VM.
//	 2) Get a VM factory. Given the ID of a VM that has been
//      registered, return the factory that the ID is associated with.
//   3) Associate a VM with an alias
//   4) Get the ID of the VM by the VM's alias
//   5) Get the aliases of a VM
type Manager interface {
	// Returns a factory that can create new instances of the VM
	// with the given ID
	GetVMFactory(ids.ID) (VMFactory, error)

	// Associate an ID with the factory that creates new instances
	// of the VM with the given ID
	RegisterVMFactory(ids.ID, VMFactory) error

	// Given an alias, return the ID of the VM associated with that alias
	Lookup(string) (ids.ID, error)

	// Return the aliases associated with a VM
	Aliases(ids.ID) []string

	// Give an alias to a VM
	Alias(ids.ID, string) error
}

// Implements Manager
type manager struct {
	// Note: The string representation of a VM's ID is also considered to be an
	// alias of the VM. That is, [VM].String() is an alias for the VM, too.
	ids.Aliaser

	// Key: The key underlying a VM's ID
	// Value: A factory that creates new instances of that VM
	vmFactories map[ids.ID]VMFactory

	// The node's API server.
	// [manager] adds routes to this server to expose new API endpoints/services
	apiServer *server.Server

	log logging.Logger
}

// NewManager returns an instance of a VM manager
func NewManager(apiServer *server.Server, log logging.Logger) Manager {
	m := &manager{
		vmFactories: make(map[ids.ID]VMFactory),
		apiServer:   apiServer,
		log:         log,
	}
	m.Initialize()
	return m
}

// Return a factory that can create new instances of the vm whose
// ID is [vmID]
func (m *manager) GetVMFactory(vmID ids.ID) (VMFactory, error) {
	if factory, ok := m.vmFactories[vmID]; ok {
		return factory, nil
	}
	return nil, fmt.Errorf("no vm with ID '%v' has been registered", vmID)

}

// Map [vmID] to [factory]. [factory] creates new instances of the vm whose
// ID is [vmID]
func (m *manager) RegisterVMFactory(vmID ids.ID, factory VMFactory) error {
	if _, exists := m.vmFactories[vmID]; exists {
		return fmt.Errorf("a vm with ID '%v' has already been registered", vmID)
	}
	if err := m.Alias(vmID, vmID.String()); err != nil {
		return err
	}

	m.vmFactories[vmID] = factory

	// add the static API endpoints
	m.addStaticAPIEndpoints(vmID)
	return nil
}

// VMs can expose a static API (one that does not depend on the state of a particular chain.)
// This method adds to the node's API server the static API of the VM with ID [vmID].
// This allows clients to call the VM's static API methods.
func (m *manager) addStaticAPIEndpoints(vmID ids.ID) {
	vmFactory, err := m.GetVMFactory(vmID)
	m.log.AssertNoError(err)
	m.log.Debug("adding static API for VM with ID %s", vmID)
	vm, err := vmFactory.New(nil)
	if err != nil {
		return
	}

	staticVM, ok := vm.(common.StaticVM)
	if !ok {
		staticVM, ok := vm.(common.VM)
		if ok {
			if err := staticVM.Shutdown(); err != nil {
				m.log.Error("shutting down static API endpoints errored with: %s", err)
			}
		}
		return
	}

	handlers, err := staticVM.CreateStaticHandlers()
	if err != nil {
		m.log.Error("starting static API endpoints for %s errored with: %s", vmID, err)

		staticVM, ok := vm.(common.VM)
		if ok {
			if err := staticVM.Shutdown(); err != nil {
				m.log.Error("shutting down static API endpoints errored with: %s", err)
			}
		}
		return
	}

	// all static endpoints go to the vm endpoint, defaulting to the vm id
	defaultEndpoint := "vm/" + vmID.String()
	// use a single lock for this entire vm
	lock := new(sync.RWMutex)
	// register the static endpoints
	for extension, service := range handlers {
		m.log.Verbo("adding static API endpoint: %s", defaultEndpoint+extension)
		if err := m.apiServer.AddRoute(service, lock, defaultEndpoint, extension, m.log); err != nil {
			m.log.Warn("failed to add static API endpoint %s: %v", fmt.Sprintf("%s%s", defaultEndpoint, extension), err)
		}
	}
}
