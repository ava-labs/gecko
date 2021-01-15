// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

type parsedFx struct {
	ID ids.ID
	Fx Fx
}

// Fx is the interface a feature extension must implement to support the AVM.
type Fx interface {
	// Initialize this feature extension to be running under this VM. Should
	// return an error if the VM is incompatible.
	Initialize(vm interface{}) error

	// Notify this Fx that the VM is in bootstrapping
	Bootstrapping() error

	// Notify this Fx that the VM is bootstrapped
	Bootstrapped() error

	// VerifyTransfer returns nil iff the given input and output are well-formed
	// and syntactically valid. Does not check signatures. That should
	// be done by calling VerifyPermission.
	VerifyTransfer(in, out interface{}) error

	// VerifyPermission returns nil iff credential [cred] allows input [in],
	// which is in transaction [tx], to spend output [out]
	VerifyPermission(tx, in, cred, out interface{}) error

	// VerifyOperation verifies that the specified transaction can spend the
	// provided utxos conditioned on the result being restricted to the provided
	// outputs. If the transaction can't spend the output based on the input and
	// credential, a non-nil error  should be returned.
	VerifyOperation(tx, op, cred interface{}, utxos []interface{}) error
}

// FxOperation ...
type FxOperation interface {
	verify.Verifiable

	Outs() []verify.State
}
