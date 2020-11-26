// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"errors"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/codec"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errOperationsNotSortedUnique = errors.New("operations not sorted and unique")
	errNoOperations              = errors.New("an operationTx must have at least one operation")
	errDoubleSpend               = errors.New("inputs attempt to double spend an input")
)

// OperationTx is a transaction with no credentials.
type OperationTx struct {
	BaseTx `serialize:"true"`
	Ops    []*Operation `serialize:"true" json:"operations"`
}

// Operations track which ops this transaction is performing. The returned array
// should not be modified.
func (t *OperationTx) Operations() []*Operation { return t.Ops }

// InputUTXOs track which UTXOs this transaction is consuming.
func (t *OperationTx) InputUTXOs() []*avax.UTXOID {
	utxos := t.BaseTx.InputUTXOs()
	for _, op := range t.Ops {
		utxos = append(utxos, op.UTXOIDs...)
	}
	return utxos
}

// ConsumedAssetIDs returns the IDs of the assets this transaction consumes
func (t *OperationTx) ConsumedAssetIDs() ids.Set {
	assets := t.BaseTx.AssetIDs()
	for _, op := range t.Ops {
		if len(op.UTXOIDs) > 0 {
			assets.Add(op.AssetID())
		}
	}
	return assets
}

// AssetIDs returns the IDs of the assets this transaction depends on
func (t *OperationTx) AssetIDs() ids.Set {
	assets := t.BaseTx.AssetIDs()
	for _, op := range t.Ops {
		assets.Add(op.AssetID())
	}
	return assets
}

// NumCredentials returns the number of expected credentials
func (t *OperationTx) NumCredentials() int { return t.BaseTx.NumCredentials() + len(t.Ops) }

// UTXOs returns the UTXOs transaction is producing.
func (t *OperationTx) UTXOs() []*avax.UTXO {
	txID := t.ID()
	utxos := t.BaseTx.UTXOs()

	for _, op := range t.Ops {
		asset := op.AssetID()
		for _, out := range op.Op.Outs() {
			utxos = append(utxos, &avax.UTXO{
				UTXOID: avax.UTXOID{
					TxID:        txID,
					OutputIndex: uint32(len(utxos)),
				},
				Asset: avax.Asset{ID: asset},
				Out:   out,
			})
		}
	}

	return utxos
}

// SyntacticVerify that this transaction is well-formed.
func (t *OperationTx) SyntacticVerify(
	ctx *snow.Context,
	c codec.Manager,
	txFeeAssetID ids.ID,
	txFee uint64,
	_ uint64,
	numFxs int,
) error {
	switch {
	case t == nil:
		return errNilTx
	case len(t.Ops) == 0:
		return errNoOperations
	}

	if err := t.BaseTx.SyntacticVerify(ctx, c, txFeeAssetID, txFee, txFee, numFxs); err != nil {
		return err
	}

	inputs := ids.Set{}
	for _, in := range t.Ins {
		inputs.Add(in.InputID())
	}

	for _, op := range t.Ops {
		if err := op.Verify(c); err != nil {
			return err
		}
		for _, utxoID := range op.UTXOIDs {
			inputID := utxoID.InputID()
			if inputs.Contains(inputID) {
				return errDoubleSpend
			}
			inputs.Add(inputID)
		}
	}
	if !isSortedAndUniqueOperations(t.Ops, c) {
		return errOperationsNotSortedUnique
	}
	return nil
}

// SemanticVerify that this transaction is well-formed.
func (t *OperationTx) SemanticVerify(vm *VM, tx UnsignedTx, creds []verify.Verifiable) error {
	if err := t.BaseTx.SemanticVerify(vm, tx, creds); err != nil {
		return err
	}

	offset := t.BaseTx.NumCredentials()
	for i, op := range t.Ops {
		cred := creds[offset+i]
		if err := vm.verifyOperation(tx, op, cred); err != nil {
			return err
		}
	}
	return nil
}
