// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/common"
)

var (
	errPending = errors.New("unexpectedly called Pending")

	_ DAGVM = &TestVM{}
)

type TestVM struct {
	common.TestVM

	CantPending, CantParse, CantGet bool

	PendingF func() []conflicts.Tx
	ParseF   func([]byte) (conflicts.Tx, error)
	GetF     func(ids.ID) (conflicts.Tx, error)
}

func (vm *TestVM) Default(cant bool) {
	vm.TestVM.Default(cant)

	vm.CantPending = cant
	vm.CantParse = cant
	vm.CantGet = cant
}

func (vm *TestVM) Pending() []conflicts.Tx {
	if vm.PendingF != nil {
		return vm.PendingF()
	}
	if vm.CantPending && vm.T != nil {
		vm.T.Fatal(errPending)
	}
	return nil
}

func (vm *TestVM) Parse(b []byte) (conflicts.Tx, error) {
	if vm.ParseF != nil {
		return vm.ParseF(b)
	}
	if vm.CantParse && vm.T != nil {
		vm.T.Fatal(errParse)
	}
	return nil, errParse
}

func (vm *TestVM) Get(txID ids.ID) (conflicts.Tx, error) {
	if vm.GetF != nil {
		return vm.GetF(txID)
	}
	if vm.CantGet && vm.T != nil {
		vm.T.Fatal(errGet)
	}
	return nil, errGet
}
