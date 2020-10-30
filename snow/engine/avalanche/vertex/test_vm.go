// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vertex

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/common"
)

var (
	errParseTx = errors.New("unexpectedly called ParseTx")
	errIssueTx = errors.New("unexpectedly called IssueTx")
	errGetTx   = errors.New("unexpectedly called GetTx")
)

// TestVM ...
type TestVM struct {
	common.TestVM

	CantPendingTxs, CantParseTx, CantIssueTx, CantGetTx bool

	PendingTxsF func() []conflicts.Tx
	ParseTxF    func([]byte) (conflicts.Tx, error)
	IssueTxF    func([]byte, func(choices.Status), func(choices.Status)) (ids.ID, error)
	GetTxF      func(ids.ID) (conflicts.Tx, error)
}

// Default ...
func (vm *TestVM) Default(cant bool) {
	vm.TestVM.Default(cant)

	vm.CantPendingTxs = cant
	vm.CantParseTx = cant
	vm.CantIssueTx = cant
	vm.CantGetTx = cant
}

// PendingTxs ...
func (vm *TestVM) PendingTxs() []conflicts.Tx {
	if vm.PendingTxsF != nil {
		return vm.PendingTxsF()
	}
	if vm.CantPendingTxs && vm.T != nil {
		vm.T.Fatalf("Unexpectedly called PendingTxs")
	}
	return nil
}

// ParseTx ...
func (vm *TestVM) ParseTx(b []byte) (conflicts.Tx, error) {
	if vm.ParseTxF != nil {
		return vm.ParseTxF(b)
	}
	if vm.CantParseTx && vm.T != nil {
		vm.T.Fatal(errParseTx)
	}
	return nil, errParseTx
}

// IssueTx ...
func (vm *TestVM) IssueTx(b []byte, issued, finalized func(choices.Status)) (ids.ID, error) {
	if vm.IssueTxF != nil {
		return vm.IssueTxF(b, issued, finalized)
	}
	if vm.CantIssueTx && vm.T != nil {
		vm.T.Fatal(errIssueTx)
	}
	return ids.ID{}, errIssueTx
}

// GetTx ...
func (vm *TestVM) GetTx(txID ids.ID) (conflicts.Tx, error) {
	if vm.GetTxF != nil {
		return vm.GetTxF(txID)
	}
	if vm.CantGetTx && vm.T != nil {
		vm.T.Fatal(errGetTx)
	}
	return nil, errGetTx
}
