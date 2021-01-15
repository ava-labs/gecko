// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package secp256k1fx

import (
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/logging"
)

var (
	txBytes  = []byte{0, 1, 2, 3, 4, 5}
	sigBytes = [crypto.SECP256K1RSigLen]byte{ // signature of addr on txBytes
		0x0e, 0x33, 0x4e, 0xbc, 0x67, 0xa7, 0x3f, 0xe8,
		0x24, 0x33, 0xac, 0xa3, 0x47, 0x88, 0xa6, 0x3d,
		0x58, 0xe5, 0x8e, 0xf0, 0x3a, 0xd5, 0x84, 0xf1,
		0xbc, 0xa3, 0xb2, 0xd2, 0x5d, 0x51, 0xd6, 0x9b,
		0x0f, 0x28, 0x5d, 0xcd, 0x3f, 0x71, 0x17, 0x0a,
		0xf9, 0xbf, 0x2d, 0xb1, 0x10, 0x26, 0x5c, 0xe9,
		0xdc, 0xc3, 0x9d, 0x7a, 0x01, 0x50, 0x9d, 0xe8,
		0x35, 0xbd, 0xcb, 0x29, 0x3a, 0xd1, 0x49, 0x32,
		0x00,
	}
	addr = ids.ShortID{
		0x01, 0x5c, 0xce, 0x6c, 0x55, 0xd6, 0xb5, 0x09,
		0x84, 0x5c, 0x8c, 0x4e, 0x30, 0xbe, 0xd9, 0x8d,
		0x39, 0x1a, 0xe7, 0xf0,
	}
	addr2     ids.ShortID
	sig2Bytes [crypto.SECP256K1RSigLen]byte // signature of addr2 on txBytes
)

func init() {
	b, err := formatting.Decode(formatting.CB58, "31SoC6ehdWUWFcuzkXci7ymFEQ8HGTJgw")
	if err != nil {
		panic(err)
	}
	copy(addr2[:], b)
	b, err = formatting.Decode(formatting.CB58, "c7doHa86hWYyfXTVnNsdP1CG1gxhXVpZ9Q5CiHi2oFRdnaxh2YR2Mvu2cUNMgyQy4BNQaXAxWWPt36BJ5pDWX1Xeos4h9L")
	if err != nil {
		panic(err)
	}
	copy(sig2Bytes[:], b)

}

func TestFxInitialize(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	fx := Fx{}
	err := fx.Initialize(&vm)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFxInitializeInvalid(t *testing.T) {
	fx := Fx{}
	err := fx.Initialize(nil)
	if err == nil {
		t.Fatalf("Should have returned an error")
	}
}

func TestFxVerifyTransfer(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapping(); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapped(); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err != nil {
		t.Fatal(err)
	}
}

func TestFxVerifyTransferNilTx(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(nil, in, cred, out); err == nil {
		t.Fatalf("Should have failed verification due to a nil tx")
	}
}

func TestFxVerifyTransferNilOutput(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}

	if err := fx.VerifyTransfer(in, nil); err == nil {
		t.Fatalf("Should have failed verification due to a nil output")
	}
}

func TestFxVerifyTransferNilInput(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}

	if err := fx.VerifyTransfer(nil, out); err == nil {
		t.Fatalf("Should have failed verification due to a nil input")
	}
}

func TestFxVerifyTransferNilCredential(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, nil, out); err == nil {
		t.Fatalf("Should have failed verification due to a nil credential")
	}
}

func TestFxVerifyTransferInvalidOutput(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 0,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}

	if err := fx.VerifyTransfer(in, out); err == nil {
		t.Fatalf("Should have errored due to an invalid output")
	}
}

func TestFxVerifyTransferWrongAmounts(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 2,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}

	if err := fx.VerifyTransfer(in, out); err == nil {
		t.Fatalf("Should have errored due to different amounts")
	}
}

func TestFxVerifyTransferTimelocked(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  uint64(date.Add(time.Second).Unix()),
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err == nil {
		t.Fatalf("Should have errored due to a timelocked output")
	}
}

func TestFxVerifyTransferTooManySigners(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0, 1},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
			{},
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err == nil {
		t.Fatalf("Should have errored due to too many signers")
	}
}

func TestFxVerifyTransferTooFewSigners(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err == nil {
		t.Fatalf("Should have errored due to too few signers")
	}
}

func TestFxVerifyTransferMismatchedSigners(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
			{},
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err == nil {
		t.Fatalf("Should have errored due to too mismatched signers")
	}
}

func TestFxVerifyTransferInvalidSignature(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapping(); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			{},
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err != nil {
		t.Fatal(err)
	}

	if err := fx.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err == nil {
		t.Fatalf("Should have errored due to an invalid signature")
	}
}

func TestFxVerifyTransferWrongSigner(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapping(); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	out := &TransferOutput{
		Amt: 1,
		OutputOwners: OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs: []ids.ShortID{
				ids.ShortEmpty,
			},
		},
	}
	in := &TransferInput{
		Amt: 1,
		Input: Input{
			SigIndices: []uint32{0},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	if err := fx.VerifyTransfer(in, out); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyPermission(tx, in, cred, out); err != nil {
		t.Fatal(err)
	}

	if err := fx.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	if err := fx.VerifyPermission(tx, in, cred, out); err == nil {
		t.Fatalf("Should have errored due to a wrong signer")
	}
}

func TestFxVerifyOperation(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{utxo}
	err := fx.VerifyOperation(tx, op, cred, utxos)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFxVerifyOperationUnknownTx(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{utxo}
	err := fx.VerifyOperation(nil, op, cred, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid tx type")
	}
}

func TestFxVerifyOperationUnknownOperation(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{utxo}
	err := fx.VerifyOperation(tx, nil, cred, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid operation type")
	}
}

func TestFxVerifyOperationUnknownCredential(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
	}

	utxos := []interface{}{utxo}
	err := fx.VerifyOperation(tx, op, nil, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid credential type")
	}
}

func TestFxVerifyOperationWrongNumberOfUTXOs(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{utxo, utxo}
	err := fx.VerifyOperation(tx, op, cred, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to a wrong number of utxos")
	}
}

func TestFxVerifyOperationUnknownUTXOType(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{nil}
	err := fx.VerifyOperation(tx, op, cred, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to an invalid utxo type")
	}
}

func TestFxVerifyOperationInvalidOperationVerify(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{utxo}
	err := fx.VerifyOperation(tx, op, cred, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to a failed verify")
	}
}

func TestFxVerifyOperationMismatchedMintOutputs(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &TestTx{Bytes: txBytes}
	utxo := &MintOutput{
		OutputOwners: OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &MintOperation{
		MintInput: Input{
			SigIndices: []uint32{0},
		},
		MintOutput: MintOutput{
			OutputOwners: OutputOwners{},
		},
		TransferOutput: TransferOutput{
			Amt: 1,
			OutputOwners: OutputOwners{
				Locktime:  0,
				Threshold: 1,
				Addrs: []ids.ShortID{
					addr,
				},
			},
		},
	}
	cred := &Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}

	utxos := []interface{}{utxo}
	err := fx.VerifyOperation(tx, op, cred, utxos)
	if err == nil {
		t.Fatalf("Should have errored due to the wrong MintOutput being created")
	}
}

func TestVerifyPermission(t *testing.T) {
	vm := TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapping(); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	type test struct {
		description string
		tx          Tx
		in          *Input
		cred        *Credential
		cg          *OutputOwners
		shouldErr   bool
	}
	tests := []test{
		{
			"threshold 0, no sigs, has addrs",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{}},
			&OutputOwners{
				Threshold: 0,
				Addrs:     []ids.ShortID{addr},
			},
			true,
		},
		{
			"threshold 0, no sigs, no addrs",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{}},
			&OutputOwners{
				Threshold: 0,
				Addrs:     []ids.ShortID{},
			},
			false,
		},
		{
			"threshold 1, 1 sig",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{0}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes}},
			&OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{addr},
			},
			false,
		},
		{
			"threshold 0, 1 sig (too many sigs)",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{0}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes}},
			&OutputOwners{
				Threshold: 0,
				Addrs:     []ids.ShortID{addr},
			},
			true,
		},
		{
			"threshold 1, 0 sigs (too few sigs)",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{}},
			&OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{addr},
			},
			true,
		},
		{
			"threshold 1, 1 incorrect sig",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{0}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes}},
			&OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{ids.GenerateTestShortID()},
			},
			true,
		},
		{
			"repeated sig",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{0, 0}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes, sigBytes}},
			&OutputOwners{
				Threshold: 2,
				Addrs:     []ids.ShortID{addr, addr2},
			},
			true,
		},
		{
			"threshold 2, repeated address and repeated sig",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{0, 1}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes, sigBytes}},
			&OutputOwners{
				Threshold: 2,
				Addrs:     []ids.ShortID{addr, addr},
			},
			true,
		},
		{
			"threshold 2, 2 sigs",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{0, 1}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes, sig2Bytes}},
			&OutputOwners{
				Threshold: 2,
				Addrs:     []ids.ShortID{addr, addr2},
			},
			false,
		},
		{
			"threshold 2, 2 sigs reversed (should be sorted)",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{1, 0}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sig2Bytes, sigBytes}},
			&OutputOwners{
				Threshold: 2,
				Addrs:     []ids.ShortID{addr, addr2},
			},
			true,
		},
		{
			"threshold 1, 1 sig, index out of bounds",
			&TestTx{Bytes: txBytes},
			&Input{SigIndices: []uint32{1}},
			&Credential{Sigs: [][crypto.SECP256K1RSigLen]byte{sigBytes}},
			&OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{addr},
			},
			true,
		},
	}

	for _, test := range tests {
		if err := fx.VerifyPermission(test.tx, test.in, test.cred, test.cg); err != nil && !test.shouldErr {
			t.Fatalf("test '%s' errored but it shouldn't have: %s", test.description, err)
		} else if err == nil && test.shouldErr {
			t.Fatalf("test '%s' should have errored but didn't", test.description)
		}
	}
}
