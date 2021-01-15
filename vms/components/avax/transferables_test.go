// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avax

import (
	"bytes"
	"testing"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestTransferableOutputVerifyNil(t *testing.T) {
	to := (*TransferableOutput)(nil)
	if err := to.Verify(); err == nil {
		t.Fatalf("Should have errored due to nil transferable output")
	}
}

func TestTransferableOutputVerifyNilFx(t *testing.T) {
	to := &TransferableOutput{Asset: Asset{ID: ids.Empty}}
	if err := to.Verify(); err == nil {
		t.Fatalf("Should have errored due to nil transferable fx output")
	}
}

func TestTransferableOutputVerify(t *testing.T) {
	assetID := ids.GenerateTestID()
	to := &TransferableOutput{
		Asset: Asset{ID: assetID},
		Out:   &TestTransferable{Val: 1},
	}
	if err := to.Verify(); err != nil {
		t.Fatal(err)
	}
	if to.Output() != to.Out {
		t.Fatalf("Should have returned the fx output")
	}
}

func TestTransferableOutputSorting(t *testing.T) {
	c := linearcodec.NewDefault()
	if err := c.RegisterType(&TestTransferable{}); err != nil {
		t.Fatal(err)
	}
	manager := codec.NewDefaultManager()
	if err := manager.RegisterCodec(1, c); err != nil {
		t.Fatal(err)
	}

	assetID1 := ids.ID{1}
	outs := []*TransferableOutput{
		{
			Asset: Asset{ID: assetID1},
			Out:   &TestTransferable{Val: 1},
		},
		{
			Asset: Asset{ID: ids.Empty},
			Out:   &TestTransferable{Val: 1},
		},
		{
			Asset: Asset{ID: assetID1},
			Out:   &TestTransferable{Val: 0},
		},
		{
			Asset: Asset{ID: ids.Empty},
			Out:   &TestTransferable{Val: 0},
		},
		{
			Asset: Asset{ID: ids.Empty},
			Out:   &TestTransferable{Val: 0},
		},
	}

	if IsSortedTransferableOutputs(outs, manager, 1) {
		t.Fatalf("Shouldn't be sorted")
	}
	SortTransferableOutputs(outs, manager, 1)
	if !IsSortedTransferableOutputs(outs, manager, 1) {
		t.Fatalf("Should be sorted")
	}
	if result := outs[0].Out.(*TestTransferable).Val; result != 0 {
		t.Fatalf("Val expected: %d ; result: %d", 0, result)
	}
	if result := outs[1].Out.(*TestTransferable).Val; result != 0 {
		t.Fatalf("Val expected: %d ; result: %d", 0, result)
	}
	if result := outs[2].Out.(*TestTransferable).Val; result != 1 {
		t.Fatalf("Val expected: %d ; result: %d", 0, result)
	}
	if result := outs[3].AssetID(); result != assetID1 {
		t.Fatalf("Val expected: %s ; result: %s", assetID1, result)
	}
	if result := outs[4].AssetID(); result != assetID1 {
		t.Fatalf("Val expected: %s ; result: %s", assetID1, result)
	}
}

func TestTransferableOutputSerialization(t *testing.T) {
	c := linearcodec.NewDefault()
	if err := c.RegisterType(&secp256k1fx.TransferOutput{}); err != nil {
		t.Fatal(err)
	}
	manager := codec.NewDefaultManager()
	if err := manager.RegisterCodec(1, c); err != nil {
		t.Fatal(err)
	}

	expected := []byte{
		// Codec version
		0x00, 0x01,
		// assetID:
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		// output:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xd4, 0x31, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x02, 0x51, 0x02, 0x5c, 0x61,
		0xfb, 0xcf, 0xc0, 0x78, 0xf6, 0x93, 0x34, 0xf8,
		0x34, 0xbe, 0x6d, 0xd2, 0x6d, 0x55, 0xa9, 0x55,
		0xc3, 0x34, 0x41, 0x28, 0xe0, 0x60, 0x12, 0x8e,
		0xde, 0x35, 0x23, 0xa2, 0x4a, 0x46, 0x1c, 0x89,
		0x43, 0xab, 0x08, 0x59,
	}

	out := &TransferableOutput{
		Asset: Asset{
			ID: ids.ID{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
				0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
				0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
				0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
			},
		},
		Out: &secp256k1fx.TransferOutput{
			Amt: 12345,
			OutputOwners: secp256k1fx.OutputOwners{
				Locktime:  54321,
				Threshold: 1,
				Addrs: []ids.ShortID{
					{
						0x51, 0x02, 0x5c, 0x61, 0xfb, 0xcf, 0xc0, 0x78,
						0xf6, 0x93, 0x34, 0xf8, 0x34, 0xbe, 0x6d, 0xd2,
						0x6d, 0x55, 0xa9, 0x55,
					},
					{
						0xc3, 0x34, 0x41, 0x28, 0xe0, 0x60, 0x12, 0x8e,
						0xde, 0x35, 0x23, 0xa2, 0x4a, 0x46, 0x1c, 0x89,
						0x43, 0xab, 0x08, 0x59,
					},
				},
			},
		},
	}

	outBytes, err := manager.Marshal(1, out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(outBytes, expected) {
		t.Fatalf("Expected:\n%s\nResult:\n%s",
			formatting.DumpBytes{Bytes: expected},
			formatting.DumpBytes{Bytes: outBytes},
		)
	}
}

func TestTransferableInputVerifyNil(t *testing.T) {
	ti := (*TransferableInput)(nil)
	if err := ti.Verify(); err == nil {
		t.Fatalf("Should have errored due to nil transferable input")
	}
}

func TestTransferableInputVerifyNilFx(t *testing.T) {
	ti := &TransferableInput{
		UTXOID: UTXOID{TxID: ids.Empty},
		Asset:  Asset{ID: ids.Empty},
	}
	if err := ti.Verify(); err == nil {
		t.Fatalf("Should have errored due to nil transferable fx input")
	}
}

func TestTransferableInputVerify(t *testing.T) {
	assetID := ids.GenerateTestID()
	ti := &TransferableInput{
		UTXOID: UTXOID{TxID: assetID},
		Asset:  Asset{ID: assetID},
		In:     &TestTransferable{},
	}
	if err := ti.Verify(); err != nil {
		t.Fatal(err)
	}
	if ti.Input() != ti.In {
		t.Fatalf("Should have returned the fx input")
	}
}

func TestTransferableInputSorting(t *testing.T) {
	c := linearcodec.NewDefault()
	if err := c.RegisterType(&TestTransferable{}); err != nil {
		t.Fatal(err)
	}

	ins := []*TransferableInput{
		{
			UTXOID: UTXOID{
				TxID:        ids.ID{1},
				OutputIndex: 1,
			},
			Asset: Asset{ID: ids.Empty},
			In:    &TestTransferable{},
		},
		{
			UTXOID: UTXOID{
				TxID:        ids.ID{1},
				OutputIndex: 0,
			},
			Asset: Asset{ID: ids.Empty},
			In:    &TestTransferable{},
		},
		{
			UTXOID: UTXOID{
				TxID:        ids.Empty,
				OutputIndex: 1,
			},
			Asset: Asset{ID: ids.Empty},
			In:    &TestTransferable{},
		},
		{
			UTXOID: UTXOID{
				TxID:        ids.Empty,
				OutputIndex: 0,
			},
			Asset: Asset{ID: ids.Empty},
			In:    &TestTransferable{},
		},
	}

	if IsSortedAndUniqueTransferableInputs(ins) {
		t.Fatalf("Shouldn't be sorted")
	}
	SortTransferableInputs(ins)
	if !IsSortedAndUniqueTransferableInputs(ins) {
		t.Fatalf("Should be sorted")
	}

	ins = append(ins, &TransferableInput{
		UTXOID: UTXOID{
			TxID:        ids.Empty,
			OutputIndex: 1,
		},
		Asset: Asset{ID: ids.Empty},
		In:    &TestTransferable{},
	})

	if IsSortedAndUniqueTransferableInputs(ins) {
		t.Fatalf("Shouldn't be unique")
	}
}

func TestTransferableInputSerialization(t *testing.T) {
	c := linearcodec.NewDefault()
	if err := c.RegisterType(&secp256k1fx.TransferInput{}); err != nil {
		t.Fatal(err)
	}
	manager := codec.NewDefaultManager()
	if err := manager.RegisterCodec(1, c); err != nil {
		t.Fatal(err)
	}

	expected := []byte{
		// Codec version
		0x00, 0x01,
		// txID:
		0xf1, 0xe1, 0xd1, 0xc1, 0xb1, 0xa1, 0x91, 0x81,
		0x71, 0x61, 0x51, 0x41, 0x31, 0x21, 0x11, 0x01,
		0xf0, 0xe0, 0xd0, 0xc0, 0xb0, 0xa0, 0x90, 0x80,
		0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, 0x00,
		// utxoIndex:
		0x00, 0x00, 0x00, 0x05,
		// assetID:
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		// input:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 0x5b, 0xcd, 0x15, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x07,
	}

	in := &TransferableInput{
		UTXOID: UTXOID{
			TxID: ids.ID{
				0xf1, 0xe1, 0xd1, 0xc1, 0xb1, 0xa1, 0x91, 0x81,
				0x71, 0x61, 0x51, 0x41, 0x31, 0x21, 0x11, 0x01,
				0xf0, 0xe0, 0xd0, 0xc0, 0xb0, 0xa0, 0x90, 0x80,
				0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, 0x00,
			},
			OutputIndex: 5,
		},
		Asset: Asset{
			ID: ids.ID{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
				0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
				0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
				0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
			},
		},
		In: &secp256k1fx.TransferInput{
			Amt: 123456789,
			Input: secp256k1fx.Input{
				SigIndices: []uint32{3, 7},
			},
		},
	}

	inBytes, err := manager.Marshal(1, in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inBytes, expected) {
		t.Fatalf("Expected:\n%s\nResult:\n%s",
			formatting.DumpBytes{Bytes: expected},
			formatting.DumpBytes{Bytes: inBytes},
		)
	}
}
