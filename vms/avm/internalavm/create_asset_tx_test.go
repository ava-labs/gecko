// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package internalavm

import (
	"bytes"
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/codec"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

var (
	nameTooLong          = "LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL"
	symbolTooLong        = "LLLLL"
	illegalNameCharacter = "h8*32"
	invalidASCIIStr      = "ÉÎ"
	invalidWhitespaceStr = " HAT"
	denominationTooLarge = byte(maxDenomination + 1)
)

func validCreateAssetTx(t *testing.T) (*CreateAssetTx, codec.Manager, *snow.Context) {
	_, c := setupCodec()
	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
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
		}},
		Name:         "NormalName",
		Symbol:       "TICK",
		Denomination: byte(2),
		States: []*InitialState{
			{
				FxID: 0,
				Outs: []verify.State{
					&secp256k1fx.TransferOutput{
						Amt: 12345,
						OutputOwners: secp256k1fx.OutputOwners{
							Threshold: 1,
							Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
						},
					},
				},
			},
		},
	}

	unsignedBytes, err := c.Marshal(codecVersion, tx)
	if err != nil {
		t.Fatal(err)
	}
	tx.Initialize(unsignedBytes, unsignedBytes)

	ctx := NewContext(t)
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err != nil {
		t.Fatalf("Valid CreateAssetTx failed syntactic verification due to: %s", err)
	}
	return tx, c, ctx
}

func TestCreateAssetTxSerialization(t *testing.T) {
	expected := []byte{
		// Codec version:
		0x00, 0x00,
		// txID:
		0x00, 0x00, 0x00, 0x01,
		// networkID:
		0x00, 0x00, 0x00, 0x02,
		// blockchainID:
		0xff, 0xff, 0xff, 0xff, 0xee, 0xee, 0xee, 0xee,
		0xdd, 0xdd, 0xdd, 0xdd, 0xcc, 0xcc, 0xcc, 0xcc,
		0xbb, 0xbb, 0xbb, 0xbb, 0xaa, 0xaa, 0xaa, 0xaa,
		0x99, 0x99, 0x99, 0x99, 0x88, 0x88, 0x88, 0x88,
		// number of outs:
		0x00, 0x00, 0x00, 0x01,
		// output[0]:
		// assetID:
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		// output:
		0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xd4, 0x31, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x02, 0x51, 0x02, 0x5c, 0x61,
		0xfb, 0xcf, 0xc0, 0x78, 0xf6, 0x93, 0x34, 0xf8,
		0x34, 0xbe, 0x6d, 0xd2, 0x6d, 0x55, 0xa9, 0x55,
		0xc3, 0x34, 0x41, 0x28, 0xe0, 0x60, 0x12, 0x8e,
		0xde, 0x35, 0x23, 0xa2, 0x4a, 0x46, 0x1c, 0x89,
		0x43, 0xab, 0x08, 0x59,
		// number of inputs:
		0x00, 0x00, 0x00, 0x01,
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
		0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00,
		0x07, 0x5b, 0xcd, 0x15, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x07,
		// Memo length:
		0x00, 0x00, 0x00, 0x04,
		// Memo:
		0x00, 0x01, 0x02, 0x03,
		// name:
		0x00, 0x10, 0x56, 0x6f, 0x6c, 0x61, 0x74, 0x69,
		0x6c, 0x69, 0x74, 0x79, 0x20, 0x49, 0x6e, 0x64,
		0x65, 0x78,
		// symbol:
		0x00, 0x03, 0x56, 0x49, 0x58,
		// denomination:
		0x02,
		// number of InitialStates:
		0x00, 0x00, 0x00, 0x01,
		// InitialStates[0]:
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xd4, 0x31, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x02, 0x51, 0x02, 0x5c, 0x61,
		0xfb, 0xcf, 0xc0, 0x78, 0xf6, 0x93, 0x34, 0xf8,
		0x34, 0xbe, 0x6d, 0xd2, 0x6d, 0x55, 0xa9, 0x55,
		0xc3, 0x34, 0x41, 0x28, 0xe0, 0x60, 0x12, 0x8e,
		0xde, 0x35, 0x23, 0xa2, 0x4a, 0x46, 0x1c, 0x89,
		0x43, 0xab, 0x08, 0x59,
		// number of credentials:
		0x00, 0x00, 0x00, 0x00,
	}

	tx := &Tx{UnsignedTx: &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID: 2,
			BlockchainID: ids.ID{
				0xff, 0xff, 0xff, 0xff, 0xee, 0xee, 0xee, 0xee,
				0xdd, 0xdd, 0xdd, 0xdd, 0xcc, 0xcc, 0xcc, 0xcc,
				0xbb, 0xbb, 0xbb, 0xbb, 0xaa, 0xaa, 0xaa, 0xaa,
				0x99, 0x99, 0x99, 0x99, 0x88, 0x88, 0x88, 0x88,
			},
			Memo: []byte{0x00, 0x01, 0x02, 0x03},
			Outs: []*avax.TransferableOutput{{
				Asset: avax.Asset{
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
							ids.NewShortID([20]byte{
								0x51, 0x02, 0x5c, 0x61, 0xfb, 0xcf, 0xc0, 0x78,
								0xf6, 0x93, 0x34, 0xf8, 0x34, 0xbe, 0x6d, 0xd2,
								0x6d, 0x55, 0xa9, 0x55,
							}),
							ids.NewShortID([20]byte{
								0xc3, 0x34, 0x41, 0x28, 0xe0, 0x60, 0x12, 0x8e,
								0xde, 0x35, 0x23, 0xa2, 0x4a, 0x46, 0x1c, 0x89,
								0x43, 0xab, 0x08, 0x59,
							}),
						},
					},
				},
			}},
			Ins: []*avax.TransferableInput{{
				UTXOID: avax.UTXOID{
					TxID: ids.ID{
						0xf1, 0xe1, 0xd1, 0xc1, 0xb1, 0xa1, 0x91, 0x81,
						0x71, 0x61, 0x51, 0x41, 0x31, 0x21, 0x11, 0x01,
						0xf0, 0xe0, 0xd0, 0xc0, 0xb0, 0xa0, 0x90, 0x80,
						0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, 0x00,
					},
					OutputIndex: 5,
				},
				Asset: avax.Asset{
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
			}},
		}},
		Name:         "Volatility Index",
		Symbol:       "VIX",
		Denomination: 2,
		States: []*InitialState{
			{
				FxID: 0,
				Outs: []verify.State{
					&secp256k1fx.TransferOutput{
						Amt: 12345,
						OutputOwners: secp256k1fx.OutputOwners{
							Locktime:  54321,
							Threshold: 1,
							Addrs: []ids.ShortID{
								ids.NewShortID([20]byte{
									0x51, 0x02, 0x5c, 0x61, 0xfb, 0xcf, 0xc0, 0x78,
									0xf6, 0x93, 0x34, 0xf8, 0x34, 0xbe, 0x6d, 0xd2,
									0x6d, 0x55, 0xa9, 0x55,
								}),
								ids.NewShortID([20]byte{
									0xc3, 0x34, 0x41, 0x28, 0xe0, 0x60, 0x12, 0x8e,
									0xde, 0x35, 0x23, 0xa2, 0x4a, 0x46, 0x1c, 0x89,
									0x43, 0xab, 0x08, 0x59,
								}),
							},
						},
					},
				},
			},
		},
	}}

	_, c := setupCodec()
	if err := tx.SignSECP256K1Fx(c, nil); err != nil {
		t.Fatal(err)
	}

	result := tx.Bytes()
	if !bytes.Equal(expected, result) {
		t.Fatalf("\nExpected: 0x%x\nResult:   0x%x", expected, result)
	}
}

func TestCreateAssetTxGetters(t *testing.T) {
	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if initialStates := tx.InitialStates(); len(initialStates) != 1 {
		t.Fatalf("Wrong number of assets returned")
	} else if initialState := initialStates[0]; initialState.FxID != 0 {
		t.Fatalf("Wrong fxID returned")
	} else if len(initialState.Outs) != 0 {
		t.Fatalf("Wrong number of outs returned")
	} else if utxos := tx.UTXOs(); len(utxos) != 0 {
		t.Fatalf("Wrong number of utxos returned")
	}
}

func TestCreateAssetTxSyntacticVerify(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAssetTxSyntacticVerifyNil(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := (*CreateAssetTx)(nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Nil CreateAssetTx should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyNameTooShort(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Too short name should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyNameTooLong(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name: "BRADY WINSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSS" +
			"SSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSSS" +
			"SSS",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Too long name should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifySymbolTooShort(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Too short symbol should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifySymbolTooLong(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "TOM",
		Symbol:       "BRADY",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Too long symbol should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyNoFxs(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: 0,
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("No Fxs should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyDenominationTooLong(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: denominationTooLarge,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Too large denomination should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyNameWithWhitespace(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY ",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Whitespace at the end of the name should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyNameWithInvalidCharacter(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY!",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Name with an invalid character should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyNameWithUnicodeCharacter(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         illegalNameCharacter,
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Name with an invalid character should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifySymbolWithInvalidCharacter(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM!",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Symbol with an invalid character should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyInvalidBaseTx(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID + 1,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 0,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Invalid BaseTx should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyInvalidInitialState(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{{
			FxID: 1,
		}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Invalid InitialState should have errored")
	}
}

func TestCreateAssetTxSyntacticVerifyUnsortedInitialStates(t *testing.T) {
	ctx := NewContext(t)
	_, c := setupCodec()

	tx := &CreateAssetTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		}},
		Name:         "BRADY",
		Symbol:       "TOM",
		Denomination: 0,
		States: []*InitialState{
			{
				FxID: 1,
			},
			{
				FxID: 0,
			},
		},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 2); err == nil {
		t.Fatalf("Unsorted InitialStates should have errored")
	}
}

func TestCreateAssetTxNotState(t *testing.T) {
	intf := interface{}(&CreateAssetTx{})
	if _, ok := intf.(verify.State); ok {
		t.Fatalf("shouldn't be marked as state")
	}
}

func TestCreateAssetTxSyntacticVerifyName(t *testing.T) {
	tx, c, ctx := validCreateAssetTx(t)

	// String of Length 129 should fail SyntacticVerify
	tx.Name = nameTooLong

	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to name too long")
	}

	tx.Name = invalidWhitespaceStr
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to invalid whitespace in name")
	}

	tx.Name = invalidASCIIStr
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to invalid ASCII character in name")
	}
}

func TestCreateAssetTxSyntacticVerifySymbol(t *testing.T) {
	tx, c, ctx := validCreateAssetTx(t)

	tx.Symbol = symbolTooLong
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to symbol too long")
	}

	tx.Symbol = " F"
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to invalid whitespace in symbol")
	}

	tx.Symbol = "É"
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to invalid ASCII character in symbol")
	}
}

func TestCreateAssetTxSyntacticVerifyInvalidDenomination(t *testing.T) {
	tx, c, ctx := validCreateAssetTx(t)

	tx.Denomination = byte(33)
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to denomination too large")
	}
}

func TestCreateAssetTxSyntacticVerifyInitialStates(t *testing.T) {
	tx, c, ctx := validCreateAssetTx(t)

	tx.States = []*InitialState{}
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to no Initial States")
	}

	tx.States = []*InitialState{
		{
			FxID: 5, // Invalid FxID
			Outs: []verify.State{
				&secp256k1fx.TransferOutput{
					Amt: 12345,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		},
	}

	// NumFxs is 1, so FxID 5 should cause an error
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 1); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to invalid Fx")
	}

	uniqueStates := []*InitialState{
		{
			FxID: 0,
			Outs: []verify.State{
				&secp256k1fx.TransferOutput{
					Amt: 12345,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		},
		{
			FxID: 1,
			Outs: []verify.State{
				&secp256k1fx.TransferOutput{
					Amt: 12345,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		},
		{
			FxID: 2,
			Outs: []verify.State{
				&secp256k1fx.TransferOutput{
					Amt: 12345,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs:     []ids.ShortID{keys[0].PublicKey().Address()},
					},
				},
			},
		},
	}

	sortInitialStates(uniqueStates)

	// Put states in unsorted order
	tx.States = []*InitialState{
		uniqueStates[2],
		uniqueStates[0],
	}
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 3); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to non-sorted initial states")
	}

	tx.States = []*InitialState{
		uniqueStates[0],
		uniqueStates[0],
	}
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 3); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to non-unique initial states")
	}

}

func TestCreateAssetTxSyntacticVerifyBaseTx(t *testing.T) {
	tx, c, ctx := validCreateAssetTx(t)
	var baseTx BaseTx
	tx.BaseTx = baseTx
	if err := tx.SyntacticVerify(ctx, c, assetID, 0, 0, 2); err == nil {
		t.Fatal("CreateAssetTx should have failed syntactic verification due to invalid BaseTx (nil)")
	}
}
