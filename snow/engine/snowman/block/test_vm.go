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
	errBuildBlock = errors.New("unexpectedly called BuildBlock")
	errParseBlock = errors.New("unexpectedly called ParseBlock")
	errGetBlock   = errors.New("unexpectedly called GetBlock")
	errSaveBlock  = errors.New("unexpectedly called SaveBlock")
)

// TestVM ...
type TestVM struct {
	common.TestVM

	CantBuildBlock,
	CantParseBlock,
	CantGetBlock,
	CantSetPreference,
	CantLastAccepted,
	CantSaveBlock bool

	BuildBlockF    func() (snowman.Block, error)
	ParseBlockF    func([]byte) (snowman.Block, error)
	GetBlockF      func(ids.ID) (snowman.Block, error)
	SetPreferenceF func(ids.ID)
	LastAcceptedF  func() ids.ID
	SaveBlockF     func(snowman.Block) error
}

// Default ...
func (vm *TestVM) Default(cant bool) {
	vm.TestVM.Default(cant)

	vm.CantBuildBlock = cant
	vm.CantParseBlock = cant
	vm.CantGetBlock = cant
	vm.CantSetPreference = cant
	vm.CantLastAccepted = cant
	vm.CantSaveBlock = cant
}

// BuildBlock ...
func (vm *TestVM) BuildBlock() (snowman.Block, error) {
	if vm.BuildBlockF != nil {
		return vm.BuildBlockF()
	}
	if vm.CantBuildBlock && vm.T != nil {
		vm.T.Fatal(errBuildBlock)
	}
	return nil, errBuildBlock
}

// ParseBlock ...
func (vm *TestVM) ParseBlock(b []byte) (snowman.Block, error) {
	if vm.ParseBlockF != nil {
		return vm.ParseBlockF(b)
	}
	if vm.CantParseBlock && vm.T != nil {
		vm.T.Fatal(errParseBlock)
	}
	return nil, errParseBlock
}

// GetBlock ...
func (vm *TestVM) GetBlock(id ids.ID) (snowman.Block, error) {
	if vm.GetBlockF != nil {
		return vm.GetBlockF(id)
	}
	if vm.CantGetBlock && vm.T != nil {
		vm.T.Fatal(errGetBlock)
	}
	return nil, errGetBlock
}

// SetPreference ...
func (vm *TestVM) SetPreference(id ids.ID) {
	if vm.SetPreferenceF != nil {
		vm.SetPreferenceF(id)
	} else if vm.CantSetPreference && vm.T != nil {
		vm.T.Fatalf("Unexpectedly called SetPreference")
	}
}

// LastAccepted ...
func (vm *TestVM) LastAccepted() ids.ID {
	if vm.LastAcceptedF != nil {
		return vm.LastAcceptedF()
	}
	if vm.CantLastAccepted && vm.T != nil {
		vm.T.Fatalf("Unexpectedly called LastAccepted")
	}
	return ids.ID{}
}

// SaveBlock ...
func (vm *TestVM) SaveBlock(blk snowman.Block) error {
	if vm.SaveBlockF != nil {
		return vm.SaveBlockF(blk)
	}
	if vm.CantSaveBlock && vm.T != nil {
		vm.T.Fatal(errSaveBlock)
	}
	return errSaveBlock
}
