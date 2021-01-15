package secp256k1fx

import (
	"errors"

	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errNilUpdateManagedAssetOperation = errors.New("UpdateManagedAssetOperation is nil")
)

// UpdateManagedAssetStatusOperation updates the status
// of a managed asset and/or mints more of this asset
type UpdateManagedAssetOperation struct {
	// Allows a managed output status to be spent
	Input `serialize:"true"`

	// New status of this managed asset
	// May be the same as the old status
	ManagedAssetStatusOutput `serialize:"true"`
}

// Verify ...
func (op *UpdateManagedAssetOperation) Verify() error {
	if op == nil {
		return errNilUpdateManagedAssetOperation
	}
	return verify.All(&op.Input, &op.ManagedAssetStatusOutput)
}

// Outs ...
func (op *UpdateManagedAssetOperation) Outs() []verify.State {
	return []verify.State{&op.ManagedAssetStatusOutput}
}
