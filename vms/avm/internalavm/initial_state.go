// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/codec"
	"github.com/ava-labs/avalanchego/vms/components/verify"
)

var (
	errNilInitialState  = errors.New("nil initial state is not valid")
	errNilFxOutput      = errors.New("nil feature extension output is not valid")
	errOutputsNotSorted = errors.New("outputs not sorted")
)

// InitialState ...
type InitialState struct {
	FxID uint32         `serialize:"true" json:"fxID"`
	Outs []verify.State `serialize:"true" json:"outputs"`
}

// Verify implements the verify.Verifiable interface
func (is *InitialState) Verify(c codec.Manager, numFxs int) error {
	switch {
	case is == nil:
		return errNilInitialState
	case is.FxID >= uint32(numFxs):
		return errUnknownFx
	}

	for _, out := range is.Outs {
		if out == nil {
			return errNilFxOutput
		}
		if err := out.Verify(); err != nil {
			return err
		}
	}
	if !isSortedState(is.Outs, c) {
		return errOutputsNotSorted
	}

	return nil
}

// Sort ...
func (is *InitialState) Sort(c codec.Manager) { sortState(is.Outs, c) }

type innerSortState struct {
	vers  []verify.State
	codec codec.Manager
}

func (vers *innerSortState) Less(i, j int) bool {
	iVer := vers.vers[i]
	jVer := vers.vers[j]

	iBytes, err := vers.codec.Marshal(codecVersion, &iVer)
	if err != nil {
		return false
	}
	jBytes, err := vers.codec.Marshal(codecVersion, &jVer)
	if err != nil {
		return false
	}
	return bytes.Compare(iBytes, jBytes) == -1
}
func (vers *innerSortState) Len() int      { return len(vers.vers) }
func (vers *innerSortState) Swap(i, j int) { v := vers.vers; v[j], v[i] = v[i], v[j] }

func sortState(vers []verify.State, c codec.Manager) {
	sort.Sort(&innerSortState{vers: vers, codec: c})
}
func isSortedState(vers []verify.State, c codec.Manager) bool {
	return sort.IsSorted(&innerSortState{vers: vers, codec: c})
}

type innerSortInitialState []*InitialState

func (iss innerSortInitialState) Less(i, j int) bool { return iss[i].FxID < iss[j].FxID }
func (iss innerSortInitialState) Len() int           { return len(iss) }
func (iss innerSortInitialState) Swap(i, j int)      { iss[j], iss[i] = iss[i], iss[j] }

func sortInitialStates(iss []*InitialState) { sort.Sort(innerSortInitialState(iss)) }
func isSortedAndUniqueInitialStates(iss []*InitialState) bool {
	return utils.IsSortedAndUnique(innerSortInitialState(iss))
}
