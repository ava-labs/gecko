// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"bytes"
	"testing"

	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
)

func newSerializer(t *testing.T, parse func([]byte) (snowstorm.Tx, error)) *Serializer {
	vm := vertex.TestVM{}
	vm.T = t
	vm.Default(true)
	vm.ParseTxF = parse

	baseDB := memdb.New()
	ctx := snow.DefaultContextTest()
	s := &Serializer{}
	s.Initialize(ctx, &vm, baseDB)
	return s
}

func TestUnknownUniqueVertexErrors(t *testing.T) {
	s := newSerializer(t, nil)

	uVtx := &uniqueVertex{
		serializer: s,
		vtxID:      ids.ID{},
	}

	status := uVtx.Status()
	if status != choices.Unknown {
		t.Fatalf("Expected vertex to have Unknown status")
	}

	_, err := uVtx.Parents()
	if err == nil {
		t.Fatalf("Parents should have produced error for unknown vertex")
	}

	_, err = uVtx.Height()
	if err == nil {
		t.Fatalf("Height should have produced error for unknown vertex")
	}

	_, err = uVtx.Txs()
	if err == nil {
		t.Fatalf("Txs should have produced an error for unknown vertex")
	}
}

func TestUniqueVertexCacheHit(t *testing.T) {
	testTx := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV: ids.ID{1},
	}}

	s := newSerializer(t, func(b []byte) (snowstorm.Tx, error) {
		if !bytes.Equal(b, []byte{0}) {
			t.Fatal("unknown tx")
		}
		return testTx, nil
	})

	vtxID := ids.ID{2}
	parentID := ids.ID{'p', 'a', 'r', 'e', 'n', 't'}
	parentIDs := []ids.ID{parentID}
	chainID := ids.ID{} // Same as chainID of serializer
	height := uint64(1)
	vtx, err := vertex.Build(
		chainID,
		height,
		0,
		parentIDs,
		[][]byte{{0}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	uVtx := &uniqueVertex{
		vtxID:      vtxID,
		serializer: s,
	}
	if err := uVtx.setVertex(vtx); err != nil {
		t.Fatalf("Failed to set vertex due to: %s", err)
	}

	newUVtx := &uniqueVertex{
		vtxID:      vtxID,
		serializer: s,
	}

	parents, err := newUVtx.Parents()
	if err != nil {
		t.Fatalf("Error while retrieving parents of known vertex")
	}
	if len(parents) != 1 {
		t.Fatalf("Parents should have length 1")
	}
	if parents[0].ID() != parentID {
		t.Fatalf("ParentID is incorrect")
	}

	newHeight, err := newUVtx.Height()
	if err != nil {
		t.Fatalf("Error while retrieving height of known vertex")
	}
	if height != newHeight {
		t.Fatalf("Vertex height should have been %d, but was: %d", height, newHeight)
	}

	txs, err := newUVtx.Txs()
	if err != nil {
		t.Fatalf("Error while retrieving txs of known vertex: %s", err)
	}
	if len(txs) != 1 {
		t.Fatalf("Incorrect number of transactions")
	}
	if txs[0] != testTx {
		t.Fatalf("Txs retrieved the wrong Tx")
	}

	if newUVtx.v != uVtx.v {
		t.Fatalf("Unique vertex failed to get corresponding vertex state from cache")
	}
}

func TestUniqueVertexCacheMiss(t *testing.T) {
	txBytes := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	testTx := &snowstorm.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV: ids.ID{1},
		},
		BytesV: txBytes,
	}
	parseTx := func(b []byte) (snowstorm.Tx, error) {
		if !bytes.Equal(txBytes, b) {
			t.Fatal("asked to parse unexpected transaction")
		}

		return testTx, nil
	}
	s := newSerializer(t, parseTx)
	parentID := ids.ID{'p', 'a', 'r', 'e', 'n', 't'}
	parentIDs := []ids.ID{parentID}
	chainID := ids.ID{}
	height := uint64(1)
	innerVertex, err := vertex.Build(
		chainID,
		height,
		0,
		parentIDs,
		[][]byte{txBytes},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	vtxID := innerVertex.ID()
	vtxBytes := innerVertex.Bytes()

	uVtx := uniqueVertex{
		vtxID:      vtxID,
		serializer: s,
	}

	// Register a cache miss
	if status := uVtx.Status(); status != choices.Unknown {
		t.Fatalf("expected status to be unknown, but found: %s", status)
	}

	// Register cache hit
	vtx, err := newUniqueVertex(s, vtxBytes)
	if err != nil {
		t.Fatal(err)
	}

	if status := vtx.Status(); status != choices.Processing {
		t.Fatalf("expected status to be processing, but found: %s", status)
	}

	if err := vtx.Verify(); err != nil {
		t.Fatal(err)
	}

	validateVertex := func(vtx *uniqueVertex, expectedStatus choices.Status) {
		if status := vtx.Status(); status != expectedStatus {
			t.Fatalf("expected status to be %s, but found: %s", expectedStatus, status)
		}

		// Call bytes first to check for regression bug
		// where it's unsafe to call Bytes or Verify directly
		// after calling Status to refresh a vertex
		if !bytes.Equal(vtx.Bytes(), vtxBytes) {
			t.Fatalf("Found unexpected vertex bytes")
		}

		vtxParents, err := vtx.Parents()
		if err != nil {
			t.Fatalf("Fetching vertex parents errored with: %s", err)
		}
		vtxHeight, err := vtx.Height()
		if err != nil {
			t.Fatalf("Fetching vertex height errored with: %s", err)
		}
		vtxTxs, err := vtx.Txs()
		if err != nil {
			t.Fatalf("Fetching vertx txs errored with: %s", err)
		}
		switch {
		case vtxHeight != height:
			t.Fatalf("Expected vertex height to be %d, but found %d", height, vtxHeight)
		case len(vtxParents) != 1:
			t.Fatalf("Expected vertex to have 1 parent, but found %d", len(vtxParents))
		case vtxParents[0].ID() != parentID:
			t.Fatalf("Found unexpected parentID: %s, expected: %s", vtxParents[0].ID(), parentID)
		case len(vtxTxs) != 1:
			t.Fatalf("Exepcted vertex to have 1 transaction, but found %d", len(vtxTxs))
		case !bytes.Equal(vtxTxs[0].Bytes(), txBytes):
			t.Fatalf("Found unexpected transaction bytes")
		}
	}

	// Replace the vertex, so that it loses reference to parents, etc.
	vtx = &uniqueVertex{
		vtxID:      vtxID,
		serializer: s,
	}

	// Check that the vertex refreshed from the cache is valid
	validateVertex(vtx, choices.Processing)

	// Check that a newly parsed vertex refreshed from the cache is valid
	vtx, err = newUniqueVertex(s, vtxBytes)
	if err != nil {
		t.Fatal(err)
	}
	validateVertex(vtx, choices.Processing)

	// Check that refreshing a vertex when it has been removed from
	// the cache works correctly

	s.state.uniqueVtx.Flush()
	vtx = &uniqueVertex{
		vtxID:      vtxID,
		serializer: s,
	}
	validateVertex(vtx, choices.Processing)

	s.state.uniqueVtx.Flush()
	vtx, err = newUniqueVertex(s, vtxBytes)
	if err != nil {
		t.Fatal(err)
	}
	validateVertex(vtx, choices.Processing)
}
