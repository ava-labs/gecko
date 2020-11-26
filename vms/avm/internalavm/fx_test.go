// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"errors"
	"testing"
)

type FxTest struct {
	T *testing.T

	CantInitialize,
	CantBootstrapping,
	CantBootstrapped,
	CantVerifyTransfer,
	CantVerifyOperation bool

	InitializeF      func(vm interface{}) error
	BootstrappingF   func() error
	BootstrappedF    func() error
	VerifyTransferF  func(tx, in, cred, utxo interface{}) error
	VerifyOperationF func(tx, op, cred interface{}, utxos []interface{}) error
}

// Default ...
func (fx *FxTest) Default(cant bool) {
	fx.CantInitialize = cant
	fx.CantBootstrapping = cant
	fx.CantBootstrapped = cant
	fx.CantVerifyTransfer = cant
	fx.CantVerifyOperation = cant
}

func (fx *FxTest) Initialize(vm interface{}) error {
	if fx.InitializeF != nil {
		return fx.InitializeF(vm)
	}
	if !fx.CantInitialize {
		return nil
	}
	if fx.T != nil {
		fx.T.Fatalf("Unexpectedly called Initialize")
	}
	return errors.New("Unexpectedly called Initialize")
}

func (fx *FxTest) Bootstrapping() error {
	if fx.BootstrappingF != nil {
		return fx.BootstrappingF()
	}
	if !fx.CantBootstrapping {
		return nil
	}
	if fx.T != nil {
		fx.T.Fatalf("Unexpectedly called Bootstrapping")
	}
	return errors.New("Unexpectedly called Bootstrapping")
}

func (fx *FxTest) Bootstrapped() error {
	if fx.BootstrappedF != nil {
		return fx.BootstrappedF()
	}
	if !fx.CantBootstrapped {
		return nil
	}
	if fx.T != nil {
		fx.T.Fatalf("Unexpectedly called Bootstrapped")
	}
	return errors.New("Unexpectedly called Bootstrapped")
}

func (fx *FxTest) VerifyTransfer(tx, in, cred, utxo interface{}) error {
	if fx.VerifyTransferF != nil {
		return fx.VerifyTransferF(tx, in, cred, utxo)
	}
	if !fx.CantVerifyTransfer {
		return nil
	}
	if fx.T != nil {
		fx.T.Fatalf("Unexpectedly called VerifyTransfer")
	}
	return errors.New("Unexpectedly called VerifyTransfer")
}

func (fx *FxTest) VerifyOperation(tx, op, cred interface{}, utxos []interface{}) error {
	if fx.VerifyOperationF != nil {
		return fx.VerifyOperationF(tx, op, cred, utxos)
	}
	if !fx.CantVerifyOperation {
		return nil
	}
	if fx.T != nil {
		fx.T.Fatalf("Unexpectedly called VerifyOperation")
	}
	return errors.New("Unexpectedly called VerifyOperation")
}
