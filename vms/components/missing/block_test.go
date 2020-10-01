// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package missing

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
)

func TestMissingBlock(t *testing.T) {
	id := ids.NewID([32]byte{255})
	mb := Block{BlkID: id}

	if blkID := mb.ID(); !blkID.Equals(id) {
		t.Fatalf("missingBlock.ID returned %s, expected %s", blkID, id)
	} else if status := mb.Status(); status != choices.Unknown {
		t.Fatalf("missingBlock.Status returned %s, expected %s", status, choices.Unknown)
	} else if parent := mb.Parent(); !parent.IsZero() {
		t.Fatalf("missingBlock.Parent returned %v, expected %v", parent, nil)
	} else if err := mb.Verify(); err == nil {
		t.Fatalf("missingBlock.Verify returned nil, expected an error")
	} else if bytes := mb.Bytes(); bytes != nil {
		t.Fatalf("missingBlock.Bytes returned %v, expected %v", bytes, nil)
	} else if err := mb.Accept(); err == nil {
		t.Fatalf("missingBlock.Accept should have returned an error")
	} else if err := mb.Reject(); err == nil {
		t.Fatalf("missingBlock.Reject should have returned an error")
	}
}
