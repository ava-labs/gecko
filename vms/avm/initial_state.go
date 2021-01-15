// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/utils"
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
func (is *InitialState) Verify(c codec.Manager, codecVersion uint16, numFxs int) error {
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
	if !isSortedState(is.Outs, c, codecVersion) {
		return errOutputsNotSorted
	}

	return nil
}

// Sort ...
func (is *InitialState) Sort(c codec.Manager, codecVersion uint16) {
	sortState(is.Outs, c, codecVersion)
}

type innerSortState struct {
	vers         []verify.State
	codecMgr     codec.Manager
	codecVersion uint16
}

func (vers *innerSortState) Less(i, j int) bool {
	iVer := vers.vers[i]
	jVer := vers.vers[j]

	iBytes, err := vers.codecMgr.Marshal(vers.codecVersion, &iVer)
	if err != nil {
		return false
	}
	jBytes, err := vers.codecMgr.Marshal(vers.codecVersion, &jVer)
	if err != nil {
		return false
	}
	return bytes.Compare(iBytes, jBytes) == -1
}
func (vers *innerSortState) Len() int      { return len(vers.vers) }
func (vers *innerSortState) Swap(i, j int) { v := vers.vers; v[j], v[i] = v[i], v[j] }

func sortState(vers []verify.State, codecMgr codec.Manager, codecVersion uint16) {
	sort.Sort(&innerSortState{
		vers:         vers,
		codecMgr:     codecMgr,
		codecVersion: codecVersion,
	})
}
func isSortedState(vers []verify.State, codecMgr codec.Manager, codecVersion uint16) bool {
	return sort.IsSorted(&innerSortState{
		vers:         vers,
		codecMgr:     codecMgr,
		codecVersion: codecVersion,
	})
}

type innerSortInitialState []*InitialState

func (iss innerSortInitialState) Less(i, j int) bool { return iss[i].FxID < iss[j].FxID }
func (iss innerSortInitialState) Len() int           { return len(iss) }
func (iss innerSortInitialState) Swap(i, j int)      { iss[j], iss[i] = iss[i], iss[j] }

func sortInitialStates(iss []*InitialState) { sort.Sort(innerSortInitialState(iss)) }
func isSortedAndUniqueInitialStates(iss []*InitialState) bool {
	return utils.IsSortedAndUnique(innerSortInitialState(iss))
}
