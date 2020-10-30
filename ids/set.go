// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ids

import (
	"strings"
)

const (
	// The minimum capacity of a set
	minSetSize = 16
)

// Set is a set of IDs
type Set map[[32]byte]bool

func (ids *Set) init(size int) {
	if *ids == nil {
		if minSetSize > size {
			size = minSetSize
		}
		*ids = make(map[[32]byte]bool, size)
	}
}

// Add all the ids to this set, if the id is already in the set, nothing happens
func (ids *Set) Add(idList ...ID) {
	ids.init(2 * len(idList))
	for _, id := range idList {
		(*ids)[*id.ID] = true
	}
}

// Union adds all the ids from the provided sets to this set.
func (ids *Set) Union(set Set) {
	ids.init(2 * set.Len())
	for id := range set {
		(*ids)[id] = true
	}
}

// Contains returns true if the set contains this id, false otherwise
func (ids *Set) Contains(id ID) bool {
	return (*ids)[*id.ID]
}

// Overlaps returns true if the intersection of the set is non-empty
func (ids *Set) Overlaps(big Set) bool {
	small := *ids
	if small.Len() > big.Len() {
		small = big
		big = *ids
	}

	for id := range small {
		if _, ok := big[id]; ok {
			return true
		}
	}
	return false
}

// Len returns the number of ids in this set
func (ids Set) Len() int { return len(ids) }

// Remove all the id from this set, if the id isn't in the set, nothing happens
func (ids *Set) Remove(idList ...ID) {
	ids.init(1)
	for _, id := range idList {
		delete(*ids, *id.ID)
	}
}

// Clear empties this set
func (ids *Set) Clear() { *ids = nil }

// List converts this set into a list
func (ids Set) List() []ID {
	idList := make([]ID, ids.Len())
	i := 0
	for id := range ids {
		idList[i] = NewID(id)
		i++
	}
	return idList
}

// CappedList returns a list of length at most [size].
// Size should be >= 0. If size < 0, returns nil.
func (ids Set) CappedList(size int) []ID {
	if size < 0 {
		return nil
	}
	if l := ids.Len(); l < size {
		size = l
	}
	i := 0
	idList := make([]ID, size)
	for id := range ids {
		if i >= size {
			break
		}
		idList[i] = NewID(id)
		i++
	}
	return idList
}

// Equals returns true if the sets contain the same elements
func (ids Set) Equals(oIDs Set) bool {
	if ids.Len() != oIDs.Len() {
		return false
	}
	for key := range oIDs {
		if !ids[key] {
			return false
		}
	}
	return true
}

// String returns the string representation of a set
func (ids Set) String() string {
	sb := strings.Builder{}
	sb.WriteString("{")
	first := true
	for idBytes := range ids {
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(NewID(idBytes).String())
	}
	sb.WriteString("}")
	return sb.String()
}
