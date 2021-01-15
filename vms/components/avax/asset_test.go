// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avax

import (
	"testing"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
)

func TestAssetVerifyNil(t *testing.T) {
	id := (*Asset)(nil)
	if err := id.Verify(); err == nil {
		t.Fatalf("Should have errored due to nil AssetID")
	}
}

func TestAssetVerifyEmpty(t *testing.T) {
	id := Asset{}
	if err := id.Verify(); err == nil {
		t.Fatalf("Should have errored due to empty AssetID")
	}
}

func TestAssetID(t *testing.T) {
	c := linearcodec.NewDefault()
	manager := codec.NewDefaultManager()
	if err := manager.RegisterCodec(1, c); err != nil {
		t.Fatal(err)
	}

	id := Asset{
		ID: ids.ID{
			0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
			0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
			0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
			0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		},
	}

	if err := id.Verify(); err != nil {
		t.Fatal(err)
	}

	bytes, err := manager.Marshal(1, &id)
	if err != nil {
		t.Fatal(err)
	}

	newID := Asset{}
	if _, err := manager.Unmarshal(bytes, &newID); err != nil {
		t.Fatal(err)
	}

	if err := newID.Verify(); err != nil {
		t.Fatal(err)
	}

	if id.AssetID() != newID.AssetID() {
		t.Fatalf("Parsing returned the wrong Asset ID")
	}
}
