// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"bytes"
	"math"
	"testing"

	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestBaseTxSerialization(t *testing.T) {
	expected := []byte{
		// Codec version:
		0x00, 0x00,
		// txID:
		0x00, 0x00, 0x00, 0x00,
		// networkID:
		0x00, 0x00, 0x00, 0x0a,
		// blockchainID:
		0x05, 0x04, 0x03, 0x02, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// number of outs:
		0x00, 0x00, 0x00, 0x01,
		// output[0]:
		// assetID:
		0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// fxID:
		0x00, 0x00, 0x00, 0x07,
		// secp256k1 Transferable Output:
		// amount:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x39,
		// locktime:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// threshold:
		0x00, 0x00, 0x00, 0x01,
		// number of addresses
		0x00, 0x00, 0x00, 0x01,
		// address[0]
		0xfc, 0xed, 0xa8, 0xf9, 0x0f, 0xcb, 0x5d, 0x30,
		0x61, 0x4b, 0x99, 0xd7, 0x9f, 0xc4, 0xba, 0xa2,
		0x93, 0x07, 0x76, 0x26,
		// number of inputs:
		0x00, 0x00, 0x00, 0x01,
		// txID:
		0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
		0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
		0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
		0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
		// utxo index:
		0x00, 0x00, 0x00, 0x01,
		// assetID:
		0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// fxID:
		0x00, 0x00, 0x00, 0x05,
		// amount:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xd4, 0x31,
		// number of signatures:
		0x00, 0x00, 0x00, 0x01,
		// signature index[0]:
		0x00, 0x00, 0x00, 0x02,
		// Memo length:
		0x00, 0x00, 0x00, 0x04,
		// Memo:
		0x00, 0x01, 0x02, 0x03,
		// Number of credentials
		0x00, 0x00, 0x00, 0x00,
	}

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 1,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
		Memo: []byte{0x00, 0x01, 0x02, 0x03},
	}}}

	_, c := setupCodec()
	if err := tx.SignSECP256K1Fx(c, nil); err != nil {
		t.Fatal(err)
	}

	result := tx.Bytes()
	if !bytes.Equal(expected, result) {
		t.Fatalf("\nExpected: 0x%x\nResult:   0x%x", expected, result)
	}
}

func TestBaseTxGetters(t *testing.T) {
	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 1,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	txID := tx.ID()

	if assets := tx.AssetIDs(); assets.Len() != 1 {
		t.Fatalf("Wrong number of assets returned")
	} else if !assets.Contains(assetID) {
		t.Fatalf("Wrong asset returned")
	} else if assets := tx.ConsumedAssetIDs(); assets.Len() != 1 {
		t.Fatalf("Wrong number of consumed assets returned")
	} else if !assets.Contains(assetID) {
		t.Fatalf("Wrong consumed asset returned")
	} else if utxos := tx.UTXOs(); len(utxos) != 1 {
		t.Fatalf("Wrong number of utxos returned")
	} else if utxo := utxos[0]; utxo.TxID != txID {
		t.Fatalf("Wrong tx ID returned")
	} else if utxoIndex := utxo.OutputIndex; utxoIndex != 0 {
		t.Fatalf("Wrong output index returned")
	} else if gotAssetID := utxo.AssetID(); gotAssetID != assetID {
		t.Fatalf("Wrong asset ID returned")
	}
}

func TestBaseTxSyntacticVerify(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
}

func TestBaseTxSyntacticVerifyMemoTooLarge(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
		Memo: make([]byte, avax.MaxMemoSize+1),
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatal("should have failed because memo is too large")
	}
}

func TestBaseTxSyntacticVerifyNil(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := (*BaseTx)(nil)
	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Nil BaseTx should have errored")
	}
}

func TestBaseTxSyntacticVerifyWrongNetworkID(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID + 1,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 1,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Wrong networkID should have errored")
	}
}

func TestBaseTxSyntacticVerifyWrongChainID(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID.Prefix(0),
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 1,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Wrong chain ID should have errored")
	}
}

func TestBaseTxSyntacticVerifyInvalidOutput(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs:         []*avax.TransferableOutput{nil},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 1,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Invalid output should have errored")
	}
}

func TestBaseTxSyntacticVerifyUnsortedOutputs(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{
			{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 2,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
			{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		},
		Ins: []*avax.TransferableInput{
			{
				UTXOID: avax.UTXOID{
					TxID: ids.ID{
						0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
						0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
						0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
						0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
					},
					OutputIndex: 1,
				},
				Asset: avax.Asset{ID: assetID},
				In: &secp256k1fx.TransferInput{
					Amt: 54321,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{2},
					},
				},
			},
		},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Unsorted outputs should have errored")
	}
}

func TestBaseTxSyntacticVerifyInvalidInput(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{nil},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Invalid input should have errored")
	}
}

func TestBaseTxSyntacticVerifyInputOverflow(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{
			{
				UTXOID: avax.UTXOID{
					TxID: ids.ID{
						0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
						0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
						0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
						0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
					},
					OutputIndex: 0,
				},
				Asset: avax.Asset{ID: assetID},
				In: &secp256k1fx.TransferInput{
					Amt: math.MaxUint64,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{2},
					},
				},
			},
			{
				UTXOID: avax.UTXOID{
					TxID: ids.ID{
						0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
						0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
						0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
						0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
					},
					OutputIndex: 1,
				},
				Asset: avax.Asset{ID: assetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{2},
					},
				},
			},
		},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Input overflow should have errored")
	}
}

func TestBaseTxSyntacticVerifyOutputOverflow(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{
			{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 2,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
			{
				Asset: avax.Asset{ID: assetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: math.MaxUint64,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 1,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Output overflow should have errored")
	}
}

func TestBaseTxSyntacticVerifyInsufficientFunds(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: math.MaxUint64,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 1,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Insufficient funds should have errored")
	}
}

func TestBaseTxSyntacticVerifyUninitialized(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: assetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: 12345,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID: ids.ID{
					0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8,
					0xf7, 0xf6, 0xf5, 0xf4, 0xf3, 0xf2, 0xf1, 0xf0,
					0xef, 0xee, 0xed, 0xec, 0xeb, 0xea, 0xe9, 0xe8,
					0xe7, 0xe6, 0xe5, 0xe4, 0xe3, 0xe2, 0xe1, 0xe0,
				},
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: 54321,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{2},
				},
			},
		}},
	}}

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 0); err == nil {
		t.Fatalf("Uninitialized tx should have errored")
	}
}

func TestBaseTxSemanticVerify(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}
	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}
	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err != nil {
		t.Fatal(err)
	}
}

func TestBaseTxSemanticVerifyUnknownFx(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	if err := vm.CodecRegistry().RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID:        genesisTx.ID(),
					OutputIndex: 1,
				},
				Asset: avax.Asset{ID: genesisTx.ID()},
				In: &secp256k1fx.TransferInput{
					Amt: startBalance,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{
							0,
						},
					},
				},
			}},
		}},
		Creds: []verify.Verifiable{
			&avax.TestVerifiable{},
		},
	}
	if err := tx.SignSECP256K1Fx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}
	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("should have errored due to an unknown feature extension")
	}
}

func TestBaseTxSemanticVerifyWrongAssetID(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	if err := vm.CodecRegistry().RegisterType(&avax.TestVerifiable{}); err != nil {
		t.Fatal(err)
	}

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}

	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("should have errored due to an asset ID mismatch")
	}
}

func TestBaseTxSemanticVerifyUnauthorizedFx(t *testing.T) {
	ctx := NewContext(t)
	vm := &VM{}
	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	fx := &FxTest{}
	fx.InitializeF = func(interface{}) error {
		return vm.CodecRegistry().RegisterType(&avax.TestTransferable{})
	}

	genesisBytes := BuildGenesisTest(t)
	issuer := make(chan common.Message, 1)
	err := vm.Initialize(
		ctx,
		memdb.New(),
		genesisBytes,
		issuer,
		[]*common.Fx{
			{
				ID: ids.Empty,
				Fx: &secp256k1fx.Fx{},
			},
			{
				ID: ids.ID{1},
				Fx: fx,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	vm.batchTimeout = 0

	err = vm.Bootstrapping()
	if err != nil {
		t.Fatal(err)
	}

	err = vm.Bootstrapped()
	if err != nil {
		t.Fatal(err)
	}

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In:    &avax.TestTransferable{},
		}},
	}}}

	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("should have errored due to an unsupported fx")
	}
}

func TestBaseTxSemanticVerifyInvalidSignature(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID:        genesisTx.ID(),
					OutputIndex: 1,
				},
				Asset: avax.Asset{ID: genesisTx.ID()},
				In: &secp256k1fx.TransferInput{
					Amt: startBalance,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{
							0,
						},
					},
				},
			}},
		}},
		Creds: []verify.Verifiable{
			&secp256k1fx.Credential{
				Sigs: [][crypto.SECP256K1RSigLen]byte{{}},
			},
		},
	}
	if err := tx.SignSECP256K1Fx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}
	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Invalid credential should have failed verification")
	}
}

func TestBaseTxSemanticVerifyMissingUTXO(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        ids.Empty,
				OutputIndex: 1,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}

	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Unknown UTXO should have failed verification")
	}
}

func TestBaseTxSemanticVerifyInvalidUTXO(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: math.MaxUint32,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}

	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Invalid UTXO should have failed verification")
	}
}

func TestBaseTxSemanticVerifyPendingInvalidUTXO(t *testing.T) {
	genesisBytes, issuer, vm, _ := GenesisVM(t)
	ctx := vm.ctx

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	pendingTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: genesisTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := pendingTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	txID, err := vm.IssueTx(pendingTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	ctx.Lock.Unlock()

	<-issuer

	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	vm.PendingTxs()

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        txID,
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}
	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Invalid UTXO should have failed verification")
	}
}

func TestBaseTxSemanticVerifyPendingWrongAssetID(t *testing.T) {
	genesisBytes, issuer, vm, _ := GenesisVM(t)
	ctx := vm.ctx

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	pendingTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: genesisTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := pendingTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	txID, err := vm.IssueTx(pendingTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	ctx.Lock.Unlock()

	<-issuer

	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	vm.PendingTxs()

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        txID,
				OutputIndex: 0,
			},
			Asset: avax.Asset{ID: assetID},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
	}}}

	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Wrong asset ID should have failed verification")
	}
}

func TestBaseTxSemanticVerifyPendingUnauthorizedFx(t *testing.T) {
	genesisBytes := BuildGenesisTest(t)
	ctx := NewContext(t)

	issuer := make(chan common.Message, 1)

	ctx.Lock.Lock()

	vm := &VM{}

	fx := &FxTest{}
	fx.InitializeF = func(interface{}) error {
		return vm.CodecRegistry().RegisterType(&avax.TestVerifiable{})
	}

	err := vm.Initialize(
		ctx,
		memdb.New(),
		genesisBytes,
		issuer,
		[]*common.Fx{
			{
				ID: ids.ID{1},
				Fx: &secp256k1fx.Fx{},
			},
			{
				ID: ids.Empty,
				Fx: fx,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	vm.batchTimeout = 0

	err = vm.Bootstrapping()
	if err != nil {
		t.Fatal(err)
	}

	err = vm.Bootstrapped()
	if err != nil {
		t.Fatal(err)
	}

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	pendingTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: genesisTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := pendingTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	txID, err := vm.IssueTx(pendingTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	ctx.Lock.Unlock()

	<-issuer

	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	vm.PendingTxs()

	tx := &Tx{
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID:        txID,
					OutputIndex: 0,
				},
				Asset: avax.Asset{ID: genesisTx.ID()},
				In: &secp256k1fx.TransferInput{
					Amt: startBalance,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{
							0,
						},
					},
				},
			}},
		}},
		Creds: []verify.Verifiable{
			&avax.TestVerifiable{},
		},
	}
	if err := tx.SignSECP256K1Fx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Unsupported feature extension should have failed verification")
	}
}

func TestBaseTxSemanticVerifyPendingInvalidSignature(t *testing.T) {
	genesisBytes := BuildGenesisTest(t)
	ctx := NewContext(t)

	issuer := make(chan common.Message, 1)

	ctx.Lock.Lock()

	vm := &VM{}

	fx := &FxTest{}
	fx.InitializeF = func(interface{}) error {
		return vm.CodecRegistry().RegisterType(&avax.TestVerifiable{})
	}

	err := vm.Initialize(
		ctx,
		memdb.New(),
		genesisBytes,
		issuer,
		[]*common.Fx{
			{
				ID: ids.ID{1},
				Fx: &secp256k1fx.Fx{},
			},
			{
				ID: ids.Empty,
				Fx: fx,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	vm.batchTimeout = 0

	err = vm.Bootstrapping()
	if err != nil {
		t.Fatal(err)
	}

	err = vm.Bootstrapped()
	if err != nil {
		t.Fatal(err)
	}

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	pendingTx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: genesisTx.ID()},
			Out: &secp256k1fx.TransferOutput{
				Amt: startBalance - vm.txFee,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Threshold: 1,
					Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
				},
			},
		}},
	}}}
	if err := pendingTx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	txID, err := vm.IssueTx(pendingTx.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	ctx.Lock.Unlock()

	<-issuer

	ctx.Lock.Lock()
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	vm.PendingTxs()

	tx := &Tx{
		UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID:        txID,
					OutputIndex: 0,
				},
				Asset: avax.Asset{ID: genesisTx.ID()},
				In: &secp256k1fx.TransferInput{
					Amt: startBalance,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{
							0,
						},
					},
				},
			}},
		}},
		Creds: []verify.Verifiable{
			&secp256k1fx.Credential{
				Sigs: [][crypto.SECP256K1RSigLen]byte{{}},
			},
		},
	}
	if err := tx.SignSECP256K1Fx(vm.codec, nil); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}
	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("Invalid signature should have failed verification")
	}
}

func TestBaseTxSemanticVerifyMalformedOutput(t *testing.T) {
	_, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	txBytes := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa8, 0x66,
		0x05, 0x04, 0x03, 0x02, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x70, 0xae, 0x33, 0xb5,
		0x60, 0x9c, 0xd8, 0x9a, 0x72, 0x92, 0x4f, 0xa2,
		0x88, 0x3f, 0x9b, 0xf1, 0xc6, 0xd8, 0x9f, 0x07,
		0x09, 0x9b, 0x2a, 0xd7, 0x1b, 0xe1, 0x7c, 0x5d,
		0x44, 0x93, 0x23, 0xdb, 0x00, 0x00, 0x00, 0x05,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc3, 0x50,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x70, 0xae, 0x33, 0xb5, 0x60, 0x9c, 0xd8, 0x9a,
		0x72, 0x92, 0x4f, 0xa2, 0x88, 0x3f, 0x9b, 0xf1,
		0xc6, 0xd8, 0x9f, 0x07, 0x09, 0x9b, 0x2a, 0xd7,
		0x1b, 0xe1, 0x7c, 0x5d, 0x44, 0x93, 0x23, 0xdb,
		0x00, 0x00, 0x00, 0x01, 0x70, 0xae, 0x33, 0xb5,
		0x60, 0x9c, 0xd8, 0x9a, 0x72, 0x92, 0x4f, 0xa2,
		0x88, 0x3f, 0x9b, 0xf1, 0xc6, 0xd8, 0x9f, 0x07,
		0x09, 0x9b, 0x2a, 0xd7, 0x1b, 0xe1, 0x7c, 0x5d,
		0x44, 0x93, 0x23, 0xdb, 0x00, 0x00, 0x00, 0x05,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc3, 0x50,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x09,
		0x00, 0x00, 0x00, 0x01, 0x50, 0x6b, 0xd9, 0x2d,
		0xe5, 0xeb, 0xc2, 0xbf, 0x8f, 0xaa, 0xf1, 0x7d,
		0xbb, 0xae, 0xb3, 0xf3, 0x13, 0x9e, 0xae, 0xb4,
		0xad, 0x32, 0x95, 0x6e, 0x92, 0x74, 0xf9, 0x53,
		0x0e, 0xcc, 0x03, 0xd8, 0x02, 0xab, 0x1c, 0x16,
		0x52, 0xd0, 0xe3, 0xfc, 0xe5, 0x93, 0xa9, 0x8e,
		0x96, 0x1e, 0x83, 0xf0, 0x12, 0x27, 0x66, 0x9f,
		0x03, 0x56, 0x9f, 0x17, 0x1b, 0xd1, 0x22, 0x90,
		0xfd, 0x64, 0xf5, 0x73, 0x01,
	}

	tx := &Tx{}
	if _, err := vm.codec.Unmarshal(txBytes, tx); err == nil {
		t.Fatalf("should have failed to unmarshal the tx")
	}
}

func TestBaseTxSemanticVerifyInvalidFxOutput(t *testing.T) {
	genesisBytes, _, vm, _ := GenesisVM(t)
	ctx := vm.ctx
	defer func() {
		if err := vm.Shutdown(); err != nil {
			t.Fatal(err)
		}
		ctx.Lock.Unlock()
	}()

	if err := vm.CodecRegistry().RegisterType(&avax.TestTransferable{}); err != nil {
		t.Fatal(err)
	}

	genesisTx := GetAVAXTxFromGenesisTest(genesisBytes, t)

	tx := &Tx{UnsignedTx: &BaseTx{BaseTx: avax.BaseTx{
		NetworkID:    networkID,
		BlockchainID: chainID,
		Ins: []*avax.TransferableInput{{
			UTXOID: avax.UTXOID{
				TxID:        genesisTx.ID(),
				OutputIndex: 2,
			},
			Asset: avax.Asset{ID: genesisTx.ID()},
			In: &secp256k1fx.TransferInput{
				Amt: startBalance,
				Input: secp256k1fx.Input{
					SigIndices: []uint32{
						0,
					},
				},
			},
		}},
		Outs: []*avax.TransferableOutput{{
			Asset: avax.Asset{ID: genesisTx.ID()},
			Out: &avax.TestTransferable{
				Val: 1,
			},
		}},
	}}}
	if err := tx.SignSECP256K1Fx(vm.codec, [][]*crypto.PrivateKeySECP256K1R{{keys[0]}}); err != nil {
		t.Fatal(err)
	}

	uTx := &UniqueTx{
		TxState: &TxState{
			Tx: tx,
		},
		vm:   vm,
		txID: tx.ID(),
	}

	if err := tx.UnsignedTx.SemanticVerify(vm, uTx.UnsignedTx, tx.Creds); err == nil {
		t.Fatalf("should have errored due to sending funds to an un-authorized fx")
	}
}

func TestBaseTxNotState(t *testing.T) {
	intf := interface{}(&BaseTx{})
	if _, ok := intf.(verify.State); ok {
		t.Fatalf("shouldn't be marked as state")
	}
}
