// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/snow/engine/common"
)

var (
	errPending = errors.New("unexpectedly called Pending")

	_ DAGVM = &TestVM{}
)

type TestVM struct {
	common.TestVM

	CantPendingTxs, CantParse, CantGet bool

	PendingTxsF func() []snowstorm.Tx
	ParseTxF    func([]byte) (snowstorm.Tx, error)
	GetTxF      func(ids.ID) (snowstorm.Tx, error)
}

func (vm *TestVM) Default(cant bool) {
	vm.TestVM.Default(cant)

	vm.CantPendingTxs = cant
	vm.CantParse = cant
	vm.CantGet = cant
}

func (vm *TestVM) PendingTxs() []snowstorm.Tx {
	if vm.PendingTxsF != nil {
		return vm.PendingTxsF()
	}
	if vm.CantPendingTxs && vm.T != nil {
		vm.T.Fatal(errPending)
	}
	return nil
}

func (vm *TestVM) ParseTx(b []byte) (snowstorm.Tx, error) {
	if vm.ParseTxF != nil {
		return vm.ParseTxF(b)
	}
	if vm.CantParse && vm.T != nil {
		vm.T.Fatal(errParse)
	}
	return nil, errParse
}

func (vm *TestVM) GetTx(txID ids.ID) (snowstorm.Tx, error) {
	if vm.GetTxF != nil {
		return vm.GetTxF(txID)
	}
	if vm.CantGet && vm.T != nil {
		vm.T.Fatal(errGet)
	}
	return nil, errGet
}
