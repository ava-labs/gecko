// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package block

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
)

var (
	errBuildBlock   = errors.New("unexpectedly called BuildBlock")
	errParseBlock   = errors.New("unexpectedly called ParseBlock")
	errGetBlock     = errors.New("unexpectedly called GetBlock")
	errLastAccepted = errors.New("unexpectedly called LastAccepted")

	_ ChainVM = &TestVM{}
)

// TestVM is a ChainVM that is useful for testing.
type TestVM struct {
	common.TestVM

	CantBuildBlock,
	CantParseBlock,
	CantGetBlock,
	CantSetPreference,
	CantLastAccepted bool

	BuildBlockF    func() (snowman.Block, error)
	ParseBlockF    func([]byte) (snowman.Block, error)
	GetBlockF      func(ids.ID) (snowman.Block, error)
	SetPreferenceF func(ids.ID) error
	LastAcceptedF  func() (ids.ID, error)
}

func (vm *TestVM) Default(cant bool) {
	vm.TestVM.Default(cant)

	vm.CantBuildBlock = cant
	vm.CantParseBlock = cant
	vm.CantGetBlock = cant
	vm.CantSetPreference = cant
	vm.CantLastAccepted = cant
}

func (vm *TestVM) BuildBlock() (snowman.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF()
	}
	if vm.CantBuildBlock && vm.T != nil {
		vm.T.Fatal(errBuildBlock)
	}
	return nil, errBuildBlock
}

func (vm *TestVM) ParseBlock(b []byte) (snowman.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(b)
	}
	if vm.CantParseBlock && vm.T != nil {
		vm.T.Fatal(errParseBlock)
	}
	return nil, errParseBlock
}

func (vm *TestVM) GetBlock(id ids.ID) (snowman.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(id)
	}
	if vm.CantGetBlock && vm.T != nil {
		vm.T.Fatal(errGetBlock)
	}
	return nil, errGetBlock
}

func (vm *TestVM) SetPreference(id ids.ID) error {
	if vm.SetPreferenceF != nil {
		return vm.SetPreferenceF(id)
	}
	if vm.CantSetPreference && vm.T != nil {
		vm.T.Fatalf("Unexpectedly called SetPreference")
	}
	return nil
}

func (vm *TestVM) LastAccepted() (ids.ID, error) {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF()
	}
	if vm.CantLastAccepted && vm.T != nil {
		vm.T.Fatalf("Unexpectedly called LastAccepted")
	}
	return ids.ID{}, errLastAccepted
}
