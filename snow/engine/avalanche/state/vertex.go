// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// maxSize is the maximum allowed vertex size. It is necessary to deter DoS.
	maxSize = 1 << 20

	// maxNumParents is the max number of parents a vertex may have
	maxNumParents = 128

	// maxTxsPerVtx is the max number of transactions a vertex may have
	maxTxsPerVtx = 128
)

const (
	version uint16 = 0
)

var (
	errBadCodec       = errors.New("invalid codec")
	errBadEpoch       = errors.New("invalid epoch")
	errExtraSpace     = errors.New("trailing buffer space")
	errInvalidParents = errors.New("vertex contains non-sorted or duplicated parentIDs")
	errInvalidTxs     = errors.New("vertex contains non-sorted or duplicated transactions")
	errNoTxs          = errors.New("vertex contains no transactions")
	errConflictingTxs = errors.New("vertex contains conflicting transactions")
)

type innerVertex struct {
	id ids.ID

	chainID ids.ID
	height  uint64

	parentIDs []ids.ID
	txs       []conflicts.Tx

	bytes []byte
}

func (vtx *innerVertex) ID() ids.ID    { return vtx.id }
func (vtx *innerVertex) Bytes() []byte { return vtx.bytes }

func (vtx *innerVertex) Verify() error {
	switch {
	case !ids.IsSortedAndUniqueIDs(vtx.parentIDs):
		return errInvalidParents
	case len(vtx.txs) == 0:
		return errNoTxs
	case !isSortedAndUniqueTxs(vtx.txs):
		return errInvalidTxs
	}

	inputIDs := ids.Set{}
	for _, tx := range vtx.txs {
		inputs := ids.Set{}
		inputs.Add(tx.InputIDs()...)
		if inputs.Overlaps(inputIDs) {
			return errConflictingTxs
		}
		inputIDs.Union(inputs)
	}

	return nil
}

/*
 * Vertex:
 * Codec        | 04 Bytes
 * Chain        | 32 Bytes
 * Height       | 08 Bytes
 * Epoch        | 04 Bytes
 * NumParents   | 04 Bytes
 * Repeated (NumParents):
 *     ParentID | 32 bytes
 * NumTxs       | 04 Bytes
 * Repeated (NumTxs):
 *     TxSize   | 04 bytes
 *     Tx       | ?? bytes
 */

// Marshal creates the byte representation of the vertex
func (vtx *innerVertex) Marshal() ([]byte, error) {
	p := wrappers.Packer{MaxSize: maxSize}

	p.PackShort(version)
	p.PackFixedBytes(vtx.chainID.Bytes())
	p.PackLong(vtx.height)
	p.PackInt(0)

	p.PackInt(uint32(len(vtx.parentIDs)))
	for _, parentID := range vtx.parentIDs {
		p.PackFixedBytes(parentID.Bytes())
	}

	p.PackInt(uint32(len(vtx.txs)))
	for _, tx := range vtx.txs {
		p.PackBytes(tx.Bytes())
	}
	return p.Bytes, p.Err
}

// Unmarshal attempts to set the contents of this vertex to the value encoded in
// the stream of bytes.
func (vtx *innerVertex) Unmarshal(b []byte, vm vertex.DAGVM) error {
	p := wrappers.Packer{Bytes: b}

	if codecID := p.UnpackShort(); codecID != version {
		p.Add(errBadCodec)
	}

	chainID, _ := ids.ToID(p.UnpackFixedBytes(hashing.HashLen))
	height := p.UnpackLong()
	if epoch := p.UnpackInt(); epoch != 0 {
		p.Add(errBadEpoch)
	}

	numParents := p.UnpackInt()
	if numParents > maxNumParents {
		return fmt.Errorf("vertex says it has %d parents but max is %d", numParents, maxNumParents)
	}
	parentIDs := make([]ids.ID, numParents)
	for i := 0; i < int(numParents) && !p.Errored(); i++ {
		parentID, err := ids.ToID(p.UnpackFixedBytes(hashing.HashLen))
		p.Add(err)
		parentIDs[i] = parentID
	}

	numTxs := p.UnpackInt()
	if numTxs > maxTxsPerVtx {
		return fmt.Errorf("vertex says it has %d txs but max is %d", numTxs, maxTxsPerVtx)
	}
	txs := make([]conflicts.Tx, numTxs)
	for i := 0; i < int(numTxs) && !p.Errored(); i++ {
		tx, err := vm.ParseTx(p.UnpackBytes())
		p.Add(err)
		txs[i] = tx
	}

	if p.Offset != len(b) {
		p.Add(errExtraSpace)
	}

	if p.Errored() {
		return p.Err
	}

	*vtx = innerVertex{
		id:        ids.NewID(hashing.ComputeHash256Array(b)),
		parentIDs: parentIDs,
		chainID:   chainID,
		height:    height,
		txs:       txs,
		bytes:     b,
	}
	return nil
}

type sortTxsData []conflicts.Tx

func (txs sortTxsData) Less(i, j int) bool {
	return bytes.Compare(txs[i].ID().Bytes(), txs[j].ID().Bytes()) == -1
}
func (txs sortTxsData) Len() int      { return len(txs) }
func (txs sortTxsData) Swap(i, j int) { txs[j], txs[i] = txs[i], txs[j] }

func sortTxs(txs []conflicts.Tx) { sort.Sort(sortTxsData(txs)) }
func isSortedAndUniqueTxs(txs []conflicts.Tx) bool {
	return utils.IsSortedAndUnique(sortTxsData(txs))
}
