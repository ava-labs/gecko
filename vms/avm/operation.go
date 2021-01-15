// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errNilOperation              = errors.New("nil operation is not valid")
	errNilFxOperation            = errors.New("nil fx operation is not valid")
	errNotSortedAndUniqueUTXOIDs = errors.New("utxo IDs not sorted and unique")
)

// Operation ...
type Operation struct {
	avax.Asset `serialize:"true"`
	UTXOIDs    []*avax.UTXOID `serialize:"true" json:"inputIDs"`
	Op         FxOperation    `serialize:"true" json:"operation"`
}

// Verify implements the verify.Verifiable interface
func (op *Operation) Verify(c codec.Manager) error {
	switch {
	case op == nil:
		return errNilOperation
	case op.Op == nil:
		return errNilFxOperation
	case !avax.IsSortedAndUniqueUTXOIDs(op.UTXOIDs):
		return errNotSortedAndUniqueUTXOIDs
	default:
		return verify.All(&op.Asset, op.Op)
	}
}

type innerSortOperation struct {
	ops          []*Operation
	codec        codec.Manager
	codecVersion uint16
}

func (ops *innerSortOperation) Less(i, j int) bool {
	iOp := ops.ops[i]
	jOp := ops.ops[j]

	iBytes, err := ops.codec.Marshal(ops.codecVersion, iOp)
	if err != nil {
		return false
	}
	jBytes, err := ops.codec.Marshal(ops.codecVersion, jOp)
	if err != nil {
		return false
	}
	return bytes.Compare(iBytes, jBytes) == -1
}
func (ops *innerSortOperation) Len() int      { return len(ops.ops) }
func (ops *innerSortOperation) Swap(i, j int) { o := ops.ops; o[j], o[i] = o[i], o[j] }

func sortOperations(ops []*Operation, c codec.Manager, codecVersion uint16) {
	sort.Sort(&innerSortOperation{
		ops:          ops,
		codec:        c,
		codecVersion: codecVersion,
	})
}
func isSortedAndUniqueOperations(ops []*Operation, c codec.Manager, codecVersion uint16) bool {
	return utils.IsSortedAndUnique(&innerSortOperation{
		ops:          ops,
		codec:        c,
		codecVersion: codecVersion,
	})
}

type innerSortOperationsWithSigners struct {
	ops          []*Operation
	signers      [][]*crypto.PrivateKeySECP256K1R
	codec        codec.Manager
	codecVersion uint16
}

func (ops *innerSortOperationsWithSigners) Less(i, j int) bool {
	iOp := ops.ops[i]
	jOp := ops.ops[j]

	iBytes, err := ops.codec.Marshal(ops.codecVersion, iOp)
	if err != nil {
		return false
	}
	jBytes, err := ops.codec.Marshal(ops.codecVersion, jOp)
	if err != nil {
		return false
	}
	return bytes.Compare(iBytes, jBytes) == -1
}
func (ops *innerSortOperationsWithSigners) Len() int { return len(ops.ops) }
func (ops *innerSortOperationsWithSigners) Swap(i, j int) {
	ops.ops[j], ops.ops[i] = ops.ops[i], ops.ops[j]
	ops.signers[j], ops.signers[i] = ops.signers[i], ops.signers[j]
}

func sortOperationsWithSigners(
	ops []*Operation,
	signers [][]*crypto.PrivateKeySECP256K1R,
	codec codec.Manager,
	codecVersion uint16,
) {
	sort.Sort(&innerSortOperationsWithSigners{
		ops:          ops,
		signers:      signers,
		codec:        codec,
		codecVersion: codecVersion,
	})
}
