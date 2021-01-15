package nftfx

import (
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/codec/linearcodec"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

var (
	txBytes  = []byte{0, 1, 2, 3, 4, 5}
	sigBytes = [crypto.SECP256K1RSigLen]byte{
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
	addr = [hashing.AddrLen]byte{
		0x01, 0x5c, 0xce, 0x6c, 0x55, 0xd6, 0xb5, 0x09,
		0x84, 0x5c, 0x8c, 0x4e, 0x30, 0xbe, 0xd9, 0x8d,
		0x39, 0x1a, 0xe7, 0xf0,
	}
)

func TestFxInitialize(t *testing.T) {
	vm := secp256k1fx.TestVM{
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

func TestFxVerifyMintOperation(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &MintOutput{OutputOwners: secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs: []ids.ShortID{
			addr,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err != nil {
		t.Fatal(err)
	}
}

func TestFxVerifyMintOperationWrongTx(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &MintOutput{OutputOwners: secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs: []ids.ShortID{
			addr,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(nil, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an invalid tx")
	}
}

func TestFxVerifyMintOperationWrongNumberUTXOs(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
	}

	utxos := []interface{}{}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to not enough utxos")
	}
}

func TestFxVerifyMintOperationWrongCredential(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	utxo := &MintOutput{OutputOwners: secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs: []ids.ShortID{
			addr,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, nil, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to a bad credential")
	}
}

func TestFxVerifyMintOperationInvalidUTXO(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
	}

	utxos := []interface{}{nil}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an invalid utxo")
	}
}

func TestFxVerifyMintOperationFailingVerification(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &MintOutput{OutputOwners: secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs: []ids.ShortID{
			addr,
			ids.ShortEmpty,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an invalid utxo output")
	}
}

func TestFxVerifyMintOperationInvalidGroupID(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &MintOutput{OutputOwners: secp256k1fx.OutputOwners{
		Threshold: 1,
		Addrs: []ids.ShortID{
			addr,
		},
	}}
	op := &MintOperation{
		MintInput: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		GroupID: 1,
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an invalid Group ID")
	}
}

func TestFxVerifyTransferOperation(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &TransferOutput{
		GroupID: 1,
		Payload: []byte{2},
		OutputOwners: secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &TransferOperation{
		Input: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		Output: TransferOutput{
			GroupID: 1,
			Payload: []byte{2},
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					ids.ShortEmpty,
				},
			},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err != nil {
		t.Fatal(err)
	}
}

func TestFxVerifyTransferOperationWrongUTXO(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	op := &TransferOperation{
		Input: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		Output: TransferOutput{
			GroupID: 1,
			Payload: []byte{2},
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					ids.ShortEmpty,
				},
			},
		},
	}

	utxos := []interface{}{nil}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an invalid utxo")
	}
}

func TestFxVerifyTransferOperationFailedVerify(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &TransferOutput{
		GroupID: 1,
		Payload: []byte{2},
		OutputOwners: secp256k1fx.OutputOwners{
			Threshold: 1,
		},
	}
	op := &TransferOperation{
		Input: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		Output: TransferOutput{
			GroupID: 1,
			Payload: []byte{2},
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					ids.ShortEmpty,
				},
			},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an invalid utxo output")
	}
}

func TestFxVerifyTransferOperationWrongGroupID(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &TransferOutput{
		GroupID: 1,
		Payload: []byte{2},
		OutputOwners: secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &TransferOperation{
		Input: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		Output: TransferOutput{
			GroupID: 2,
			Payload: []byte{2},
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					ids.ShortEmpty,
				},
			},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to a wrong unique id")
	}
}

func TestFxVerifyTransferOperationWrongBytes(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &TransferOutput{
		GroupID: 1,
		Payload: []byte{2},
		OutputOwners: secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &TransferOperation{
		Input: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		Output: TransferOutput{
			GroupID: 1,
			Payload: []byte{3},
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					ids.ShortEmpty,
				},
			},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to the wrong hash being produced")
	}
}

func TestFxVerifyTransferOperationTooSoon(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &TransferOutput{
		GroupID: 1,
		Payload: []byte{2},
		OutputOwners: secp256k1fx.OutputOwners{
			Locktime:  vm.CLK.Unix() + 1,
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}
	op := &TransferOperation{
		Input: secp256k1fx.Input{
			SigIndices: []uint32{0},
		},
		Output: TransferOutput{
			GroupID: 1,
			Payload: []byte{2},
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs: []ids.ShortID{
					ids.ShortEmpty,
				},
			},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, op, cred, utxos); err == nil {
		t.Fatalf("Should have errored due to locktime")
	}
}

func TestFxVerifyOperationUnknownOperation(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	tx := &secp256k1fx.TestTx{
		Bytes: txBytes,
	}
	cred := &Credential{Credential: secp256k1fx.Credential{
		Sigs: [][crypto.SECP256K1RSigLen]byte{
			sigBytes,
		},
	}}
	utxo := &TransferOutput{
		GroupID: 1,
		Payload: []byte{2},
		OutputOwners: secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs: []ids.ShortID{
				addr,
			},
		},
	}

	utxos := []interface{}{utxo}
	if err := fx.VerifyOperation(tx, nil, cred, utxos); err == nil {
		t.Fatalf("VerifyOperation should have errored due to an unknown operation")
	}
}

func TestFxVerifyTransfer(t *testing.T) {
	vm := secp256k1fx.TestVM{
		Codec: linearcodec.NewDefault(),
		Log:   logging.NoLog{},
	}
	date := time.Date(2019, time.January, 19, 16, 25, 17, 3, time.UTC)
	vm.CLK.Set(date)

	fx := Fx{}
	if err := fx.Initialize(&vm); err != nil {
		t.Fatal(err)
	}
	if err := fx.VerifyTransfer(nil, nil); err == nil {
		t.Fatalf("this Fx doesn't support transfers")
	}
}
