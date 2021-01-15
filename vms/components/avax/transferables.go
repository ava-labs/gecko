// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avax

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errNilTransferableOutput   = errors.New("nil transferable output is not valid")
	errNilTransferableFxOutput = errors.New("nil transferable feature extension output is not valid")
	errOutputsNotSorted        = errors.New("outputs not sorted")

	errNilTransferableInput   = errors.New("nil transferable input is not valid")
	errNilTransferableFxInput = errors.New("nil transferable feature extension input is not valid")
	errInputsNotSortedUnique  = errors.New("inputs not sorted and unique")
)

// Amounter is a data structure that has an amount of something associated with it
type Amounter interface {
	// Amount returns how much value this element represents of the asset in its
	// transaction.
	Amount() uint64
}

// TransferableIn is the interface a feature extension must provide to transfer
// value between features extensions.
type TransferableIn interface {
	verify.Verifiable
	Amounter
}

// TransferableOut is the interface a feature extension must provide to transfer
// value between features extensions.
type TransferableOut interface {
	verify.State
	Amounter
}

// TransferableOutput ...
type TransferableOutput struct {
	Asset `serialize:"true"`

	Out TransferableOut `serialize:"true" json:"output"`
}

// Output returns the feature extension output that this Output is using.
func (out *TransferableOutput) Output() TransferableOut { return out.Out }

// Verify implements the verify.Verifiable interface
func (out *TransferableOutput) Verify() error {
	switch {
	case out == nil:
		return errNilTransferableOutput
	case out.Out == nil:
		return errNilTransferableFxOutput
	default:
		return verify.All(&out.Asset, out.Out)
	}
}

type innerSortTransferableOutputs struct {
	outs         []*TransferableOutput
	codec        codec.Manager
	codecVersion uint16
}

func (outs *innerSortTransferableOutputs) Less(i, j int) bool {
	iOut := outs.outs[i]
	jOut := outs.outs[j]

	iAssetID := iOut.AssetID()
	jAssetID := jOut.AssetID()

	switch bytes.Compare(iAssetID[:], jAssetID[:]) {
	case -1:
		return true
	case 1:
		return false
	}

	iBytes, err := outs.codec.Marshal(outs.codecVersion, &iOut.Out)
	if err != nil {
		return false
	}
	jBytes, err := outs.codec.Marshal(outs.codecVersion, &jOut.Out)
	if err != nil {
		return false
	}
	return bytes.Compare(iBytes, jBytes) == -1
}
func (outs *innerSortTransferableOutputs) Len() int      { return len(outs.outs) }
func (outs *innerSortTransferableOutputs) Swap(i, j int) { o := outs.outs; o[j], o[i] = o[i], o[j] }

// SortTransferableOutputs sorts output objects
func SortTransferableOutputs(outs []*TransferableOutput, c codec.Manager, codecVersion uint16) {
	sort.Sort(&innerSortTransferableOutputs{
		outs:         outs,
		codec:        c,
		codecVersion: codecVersion,
	})
}

// IsSortedTransferableOutputs returns true if output objects are sorted
func IsSortedTransferableOutputs(outs []*TransferableOutput, c codec.Manager, codecVersion uint16) bool {
	return sort.IsSorted(&innerSortTransferableOutputs{
		outs:         outs,
		codec:        c,
		codecVersion: codecVersion,
	})
}

// TransferableInput ...
type TransferableInput struct {
	UTXOID `serialize:"true"`
	Asset  `serialize:"true"`

	In TransferableIn `serialize:"true" json:"input"`
}

// Input returns the feature extension input that this Input is using.
func (in *TransferableInput) Input() TransferableIn { return in.In }

// Verify implements the verify.Verifiable interface
func (in *TransferableInput) Verify() error {
	switch {
	case in == nil:
		return errNilTransferableInput
	case in.In == nil:
		return errNilTransferableFxInput
	default:
		return verify.All(&in.UTXOID, &in.Asset, in.In)
	}
}

type innerSortTransferableInputs []*TransferableInput

func (ins innerSortTransferableInputs) Less(i, j int) bool {
	iID, iIndex := ins[i].InputSource()
	jID, jIndex := ins[j].InputSource()

	switch bytes.Compare(iID[:], jID[:]) {
	case -1:
		return true
	case 0:
		return iIndex < jIndex
	default:
		return false
	}
}
func (ins innerSortTransferableInputs) Len() int      { return len(ins) }
func (ins innerSortTransferableInputs) Swap(i, j int) { ins[j], ins[i] = ins[i], ins[j] }

// SortTransferableInputs ...
func SortTransferableInputs(ins []*TransferableInput) { sort.Sort(innerSortTransferableInputs(ins)) }

// IsSortedAndUniqueTransferableInputs ...
func IsSortedAndUniqueTransferableInputs(ins []*TransferableInput) bool {
	return utils.IsSortedAndUnique(innerSortTransferableInputs(ins))
}

type innerSortTransferableInputsWithSigners struct {
	ins     []*TransferableInput
	signers [][]*crypto.PrivateKeySECP256K1R
}

func (ins *innerSortTransferableInputsWithSigners) Less(i, j int) bool {
	iID, iIndex := ins.ins[i].InputSource()
	jID, jIndex := ins.ins[j].InputSource()

	switch bytes.Compare(iID[:], jID[:]) {
	case -1:
		return true
	case 0:
		return iIndex < jIndex
	default:
		return false
	}
}
func (ins *innerSortTransferableInputsWithSigners) Len() int { return len(ins.ins) }
func (ins *innerSortTransferableInputsWithSigners) Swap(i, j int) {
	ins.ins[j], ins.ins[i] = ins.ins[i], ins.ins[j]
	ins.signers[j], ins.signers[i] = ins.signers[i], ins.signers[j]
}

// SortTransferableInputsWithSigners sorts the inputs and signers based on the
// input's utxo ID
func SortTransferableInputsWithSigners(ins []*TransferableInput, signers [][]*crypto.PrivateKeySECP256K1R) {
	sort.Sort(&innerSortTransferableInputsWithSigners{ins: ins, signers: signers})
}

// IsSortedAndUniqueTransferableInputsWithSigners returns true if the inputs are
// sorted and unique
func IsSortedAndUniqueTransferableInputsWithSigners(ins []*TransferableInput, signers [][]*crypto.PrivateKeySECP256K1R) bool {
	return utils.IsSortedAndUnique(&innerSortTransferableInputsWithSigners{ins: ins, signers: signers})
}

// VerifyTx verifies that the inputs and outputs flowcheck, including a fee.
// Additionally, this verifies that the inputs and outputs are sorted.
func VerifyTx(
	feeAmount uint64,
	feeAssetID ids.ID,
	allIns [][]*TransferableInput,
	allOuts [][]*TransferableOutput,
	c codec.Manager,
	codecVersion uint16,
) error {
	fc := NewFlowChecker()

	fc.Produce(feeAssetID, feeAmount) // The txFee must be burned

	// Add all the outputs to the flow checker and make sure they are sorted
	for _, outs := range allOuts {
		for _, out := range outs {
			if err := out.Verify(); err != nil {
				return err
			}
			fc.Produce(out.AssetID(), out.Output().Amount())
		}
		if !IsSortedTransferableOutputs(outs, c, codecVersion) {
			return errOutputsNotSorted
		}
	}

	// Add all the inputs to the flow checker and make sure they are sorted
	for _, ins := range allIns {
		for _, in := range ins {
			if err := in.Verify(); err != nil {
				return err
			}
			fc.Consume(in.AssetID(), in.Input().Amount())
		}
		if !IsSortedAndUniqueTransferableInputs(ins) {
			return errInputsNotSortedUnique
		}
	}

	return fc.Verify()
}
