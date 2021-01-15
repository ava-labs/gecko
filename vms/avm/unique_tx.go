// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"errors"
	"fmt"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/vms/components/avax"
)

var (
	errAssetIDMismatch = errors.New("asset IDs in the input don't match the utxo")
	errWrongAssetID    = errors.New("asset ID must be AVAX in the atomic tx")
	errMissingUTXO     = errors.New("missing utxo")
	errUnknownTx       = errors.New("transaction is unknown")
	errRejectedTx      = errors.New("transaction is rejected")
)

// UniqueTx provides a de-duplication service for txs. This only provides a
// performance boost
type UniqueTx struct {
	*TxState

	vm   *VM
	txID ids.ID
}

// TxState ...
type TxState struct {
	*Tx

	syntacticVerfication, semanticVerification map[uint32]error
	unique                                     bool

	inputs     []ids.ID
	inputUTXOs []*avax.UTXOID
	utxos      []*avax.UTXO
	deps       []ids.ID
	setDeps    bool

	status choices.Status
	epoch  uint32
}

// newUniqueTx returns the UniqueTx representation of transaction [txID].
// [rawTx] may be nil.
func newUniqueTx(vm *VM, txID ids.ID, rawTx *Tx) *UniqueTx {
	return &UniqueTx{
		vm:   vm,
		txID: txID,
		TxState: &TxState{
			Tx:                   rawTx,
			syntacticVerfication: make(map[uint32]error),
			semanticVerification: make(map[uint32]error),
		},
	}
}

func (tx *UniqueTx) refresh() {
	tx.vm.numTxRefreshes.Inc()

	if tx.TxState == nil {
		tx.TxState = &TxState{
			syntacticVerfication: make(map[uint32]error),
			semanticVerification: make(map[uint32]error),
		}
	}
	if tx.unique {
		return
	}
	unique := tx.vm.state.UniqueTx(tx)
	prevTx := tx.Tx
	if unique == tx {
		tx.vm.numTxRefreshMisses.Inc()

		txID := tx.ID()

		// If no one was in the cache, make sure that there wasn't an
		// intermediate object whose state I must reflect
		if status, err := tx.vm.state.Status(txID); err == nil {
			tx.status = status
		}
		if tx.status == choices.Accepted {
			if epoch, err := tx.vm.state.Epoch(txID); err == nil {
				tx.epoch = epoch
			}
		}
		tx.unique = true
	} else {
		tx.vm.numTxRefreshHits.Inc()

		// If someone is in the cache, they must be up to date

		// This ensures that every unique tx object points to the same tx state
		tx.TxState = unique.TxState
	}

	if tx.Tx != nil {
		return
	}

	if prevTx == nil {
		// TODO: register hits/misses for this
		if innerTx, err := tx.vm.state.Tx(tx.ID()); err == nil {
			tx.Tx = innerTx
		}
	} else {
		tx.Tx = prevTx
	}
}

// Evict is called when this UniqueTx will no longer be returned from a cache
// lookup
func (tx *UniqueTx) Evict() {
	// Lock is already held here
	tx.unique = false
	tx.deps = nil
	tx.setDeps = false
}

func (tx *UniqueTx) setStatus(status choices.Status) error {
	tx.refresh()
	if tx.status == status {
		return nil
	}
	tx.status = status
	return tx.vm.state.SetStatus(tx.ID(), status)
}

// ID returns the wrapped txID
func (tx *UniqueTx) ID() ids.ID { return tx.txID }

// Accept is called when the transaction was finalized as accepted by consensus
func (tx *UniqueTx) Accept(epoch uint32) error {
	tx.vm.ctx.Log.Debug("Accepting tx %s into epoch %d", tx.txID, epoch)

	if s := tx.Status(); s != choices.Processing {
		tx.vm.ctx.Log.Error("Failed to accept tx %s because the tx is in state %s", tx.txID, s)
		return fmt.Errorf("transaction has invalid status: %s", s)
	}

	defer tx.vm.db.Abort()

	// Remove spent utxos
	for _, utxo := range tx.InputUTXOs() {
		if utxo.Symbolic() {
			// If the UTXO is symbolic, it can't be spent
			continue
		}
		utxoID := utxo.InputID()
		if err := tx.vm.state.SpendUTXO(utxoID); err != nil {
			tx.vm.ctx.Log.Error("Failed to spend utxo %s due to %s", utxoID, err)
			return err
		}
	}

	// Add new utxos
	for _, utxo := range tx.UTXOs() {
		if err := tx.vm.state.FundUTXO(utxo); err != nil {
			tx.vm.ctx.Log.Error("Failed to fund utxo %s due to %s", utxo.InputID(), err)
			return fmt.Errorf("couldn't fund UTXO: %w", err)
		}
		if out, ok := utxo.Out.(ManagedAssetStatus); ok {
			if err := tx.vm.state.PutManagedAssetStatus(utxo.AssetID(), epoch, out); err != nil {
				return fmt.Errorf("couldn't update asset status: %w", err)
			}
		}
	}

	if err := tx.setStatus(choices.Accepted); err != nil {
		tx.vm.ctx.Log.Error("Failed to accept tx %s due to %s", tx.txID, err)
		return err
	}

	tx.epoch = epoch
	if err := tx.vm.state.SetEpoch(tx.ID(), epoch); err != nil {
		tx.vm.ctx.Log.Error("Failed to accept tx %s due to %s", tx.txID, err)
		return err
	}

	txID := tx.ID()
	commitBatch, err := tx.vm.db.CommitBatch()
	if err != nil {
		tx.vm.ctx.Log.Error("Failed to calculate CommitBatch for %s due to %s", txID, err)
		return err
	}

	if err := tx.ExecuteWithSideEffects(tx.vm, commitBatch); err != nil {
		tx.vm.ctx.Log.Error("Failed to commit accept %s due to %s", txID, err)
		return err
	}

	tx.vm.ctx.Log.Verbo("Accepted Tx: %s", txID)

	tx.vm.pubsub.Publish("accepted", txID)
	tx.vm.walletService.decided(txID)

	// Needed to prevent a memory leak
	// No need to mark [setDeps] as false since
	// there are no remaining dependencies after
	// the transaction has been accepted.
	tx.deps = nil
	tx.setDeps = false

	return nil
}

// Reject is called when the transaction was rejected by consensus in the
// provided epoch
func (tx *UniqueTx) Reject(uint32) error {
	tx.deps = nil // Needed to prevent a memory leak
	tx.setDeps = false
	return nil
}

// Status returns the current status of this transaction
func (tx *UniqueTx) Status() choices.Status {
	tx.refresh()
	return tx.status
}

// Epoch returns the epoch of this transaction
func (tx *UniqueTx) Epoch() uint32 {
	tx.refresh()
	return tx.epoch
}

// Dependencies returns the set of transactions this transaction builds on
func (tx *UniqueTx) Dependencies() []ids.ID {
	tx.refresh()

	if tx.Tx == nil {
		return nil
	}

	if tx.setDeps {
		if len(tx.deps) == 0 {
			return nil
		}
		deps := tx.deps
		tx.deps = make([]ids.ID, 0, len(deps))
		for _, depID := range deps {
			dependentTx := newUniqueTx(tx.vm, depID, nil)
			if status := dependentTx.Status(); status != choices.Accepted {
				tx.deps = append(tx.deps, depID)
			}
		}
		return tx.deps
	}

	txIDs := ids.Set{}
	for _, in := range tx.InputUTXOs() {
		if in.Symbolic() {
			continue
		}
		txID, _ := in.InputSource()
		if txIDs.Contains(txID) {
			continue
		}
		dependentTx := newUniqueTx(tx.vm, txID, nil)
		if status := dependentTx.Status(); status != choices.Accepted {
			txIDs.Add(txID)
			tx.deps = append(tx.deps, txID)
		}
	}
	tx.setDeps = true
	return tx.deps
}

// InputIDs returns the set of utxoIDs this transaction consumes
func (tx *UniqueTx) InputIDs() []ids.ID {
	tx.refresh()
	if tx.Tx == nil || len(tx.inputs) != 0 {
		return tx.inputs
	}

	inputUTXOs := tx.InputUTXOs()
	tx.inputs = make([]ids.ID, len(inputUTXOs))
	for i, utxo := range inputUTXOs {
		tx.inputs[i] = utxo.InputID()
	}
	return tx.inputs
}

// InputUTXOs returns the utxos that will be consumed on tx acceptance
func (tx *UniqueTx) InputUTXOs() []*avax.UTXOID {
	tx.refresh()
	if tx.Tx == nil || len(tx.inputUTXOs) != 0 {
		return tx.inputUTXOs
	}
	tx.inputUTXOs = tx.Tx.InputUTXOs()
	return tx.inputUTXOs
}

// UTXOs returns the utxos that will be added to the UTXO set on tx acceptance
func (tx *UniqueTx) UTXOs() []*avax.UTXO {
	tx.refresh()
	if tx.Tx == nil || len(tx.utxos) != 0 {
		return tx.utxos
	}
	tx.utxos = tx.Tx.UTXOs()
	return tx.utxos
}

// Bytes returns the binary representation of this transaction
func (tx *UniqueTx) Bytes() []byte {
	tx.refresh()
	return tx.Tx.Bytes()
}

func (tx *UniqueTx) verifyWithoutCacheWrites(epoch uint32) error {
	switch status := tx.Status(); status {
	case choices.Unknown:
		return errUnknownTx
	case choices.Accepted:
		if epoch != tx.Epoch() {
			return errRejectedTx
		}
		return nil
	case choices.Rejected:
		return errRejectedTx
	default:
		return tx.SemanticVerify(epoch)
	}
}

// Verify the validity of this transaction
func (tx *UniqueTx) Verify(epoch uint32) error {
	if err := tx.verifyWithoutCacheWrites(epoch); err != nil {
		return err
	}

	tx.semanticVerification[epoch] = nil
	tx.vm.pubsub.Publish("verified", tx.ID())
	return nil
}

// SyntacticVerify verifies that this transaction is well formed
func (tx *UniqueTx) SyntacticVerify(epoch uint32) error {
	if tx.Tx == nil {
		return errUnknownTx
	}

	verified, exists := tx.syntacticVerfication[epoch]
	if exists {
		return verified
	}

	validity := tx.Tx.SyntacticVerify(
		tx.vm.ctx,
		tx.vm.codec,
		tx.vm.ctx.AVAXAssetID,
		tx.vm.txFee,
		tx.vm.creationTxFee,
		len(tx.vm.fxs),
		epoch,
	)
	tx.syntacticVerfication[epoch] = validity
	return validity
}

// SemanticVerify the validity of this transaction
func (tx *UniqueTx) SemanticVerify(epoch uint32) error {
	validity := tx.SyntacticVerify(epoch)

	if validity != nil {
		return validity
	}

	validity, exists := tx.semanticVerification[epoch]
	if exists {
		return validity
	}

	return tx.Tx.SemanticVerify(tx.vm, tx.UnsignedTx, epoch)
}
