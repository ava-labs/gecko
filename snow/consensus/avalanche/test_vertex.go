// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
)

// TestVertex is a useful test vertex
type TestVertex struct {
	choices.TestDecidable

	ParentsV    []Vertex
	ParentsErrV error
	HeightV     uint64
	HeightErrV  error
	EpochV      uint32
	EpochErrV   error
	TxsV        []conflicts.Tx
	TxsErrV     error
	BytesV      []byte
}

// Parents implements the Vertex interface
func (v *TestVertex) Parents() ([]Vertex, error) { return v.ParentsV, v.ParentsErrV }

// Height implements the Vertex interface
func (v *TestVertex) Height() (uint64, error) { return v.HeightV, v.HeightErrV }

// Epoch implements the Vertex interface
func (v *TestVertex) Epoch() (uint32, error) { return v.EpochV, v.EpochErrV }

// Txs implements the Vertex interface
func (v *TestVertex) Txs() ([]conflicts.Tx, error) { return v.TxsV, v.TxsErrV }

// Bytes implements the Vertex interface
func (v *TestVertex) Bytes() []byte { return v.BytesV }
