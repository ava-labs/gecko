// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ids

import (
	"encoding/binary"
)

var (
	offset = uint64(0)
)

// GenerateTestID returns a new ID that should only be used for testing
func GenerateTestID() ID {
	offset++
	var b [32]byte
	binary.PutUvarint(b[:], offset)
	return ID(b)
}

// GenerateTestShortID returns a new ID that should only be used for testing
func GenerateTestShortID() ShortID {
	newID := GenerateTestID()
	newShortID, _ := ToShortID(newID[:20])
	return newShortID
}
