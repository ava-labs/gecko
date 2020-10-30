// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

func TestVertexVerify(t *testing.T) {
	conflictingInputID := ids.NewID([32]byte{'i', 'n'})
	inputs := []ids.ID{conflictingInputID}
	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV: ids.NewID([32]byte{'t', 'x', '0'}),
		},
		DependenciesV: nil,
		InputIDsV:     inputs,
	}
	validVertex := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{ids.NewID([32]byte{2})},
		txs:       []conflicts.Tx{tx0},
	}

	if err := validVertex.Verify(); err != nil {
		t.Fatalf("Valid vertex failed verification due to: %s", err)
	}

	nonUniqueParentsVtx := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{ids.NewID([32]byte{'d', 'u', 'p'}), ids.NewID([32]byte{'d', 'u', 'p'})},
		txs:       []conflicts.Tx{tx0},
	}

	if err := nonUniqueParentsVtx.Verify(); err == nil {
		t.Fatal("Vertex with non unique parents should not have passed verification")
	}

	parent0 := ids.NewID([32]byte{0})
	parent1 := ids.NewID([32]byte{1})
	sortedParents := []ids.ID{parent0, parent1}
	ids.SortIDs(sortedParents)
	nonSortedParentsVtx := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{sortedParents[1], sortedParents[0]},
		txs:       []conflicts.Tx{tx0},
	}

	if err := nonSortedParentsVtx.Verify(); err == nil {
		t.Fatal("Vertex with non-sorted parents should not have passed verification")
	}

	noTxsVertex := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{ids.NewID([32]byte{2})},
		txs:       []conflicts.Tx{},
	}

	if err := noTxsVertex.Verify(); err == nil {
		t.Fatal("Vertex with no txs should not have passed verification")
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV: ids.NewID([32]byte{'t', 'x', '1'}),
		},
		DependenciesV: nil,
		InputIDsV:     nil,
	}
	sortedTxs := []conflicts.Tx{tx0, tx1}
	sortTxs(sortedTxs)
	unsortedTxsVertex := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{ids.NewID([32]byte{2})},
		txs:       []conflicts.Tx{sortedTxs[1], sortedTxs[0]},
	}

	if err := unsortedTxsVertex.Verify(); err == nil {
		t.Fatal("Vertex with unsorted transactions should not have passed verification")
	}

	nonUniqueTxsVertex := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{ids.NewID([32]byte{2})},
		txs:       []conflicts.Tx{tx0, tx0},
	}

	if err := nonUniqueTxsVertex.Verify(); err == nil {
		t.Fatal("Vertex with non-unique transactions should not have passed verification")
	}

	inputs = append(inputs, ids.NewID([32]byte{'e', 'x', 't', 'r', 'a'}))
	conflictingTx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV: ids.NewID([32]byte{'c', 'o', 'n', 'f', 'l', 'i', 'c', 't'}),
		},
		DependenciesV: nil,
		InputIDsV:     inputs,
	}

	conflictingTxs := []conflicts.Tx{tx0, conflictingTx}
	sortTxs(conflictingTxs)

	conflictingTxsVertex := &innerVertex{
		id:        ids.NewID([32]byte{}),
		chainID:   ids.NewID([32]byte{1}),
		height:    1,
		parentIDs: []ids.ID{ids.NewID([32]byte{2})},
		txs:       conflictingTxs,
	}

	if err := conflictingTxsVertex.Verify(); err == nil {
		t.Fatal("Vertex with conflicting transactions should not have passed verification")
	}
}
