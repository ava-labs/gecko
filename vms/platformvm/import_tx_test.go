package platformvm

import (
	"math/rand"
	"testing"

	"github.com/ava-labs/avalanchego/chains/atomic"
	"github.com/ava-labs/avalanchego/database/prefixdb"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestNewImportTx(t *testing.T) {
	vm, baseDB := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	type test struct {
		description   string
		sharedMemory  atomic.SharedMemory
		recipientKeys []*crypto.PrivateKeySECP256K1R
		shouldErr     bool
	}

	factory := crypto.FactorySECP256K1R{}
	recipientKeyIntf, err := factory.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	recipientKey := recipientKeyIntf.(*crypto.PrivateKeySECP256K1R)

	cnt := new(byte)

	// Returns a shared memory where GetDatabase returns a database
	// where [recipientKey] has a balance of [amt]
	fundedSharedMemory := func(amt uint64) atomic.SharedMemory {
		*cnt++
		m := &atomic.Memory{}
		m.Initialize(logging.NoLog{}, prefixdb.New([]byte{*cnt}, baseDB))

		sm := m.NewSharedMemory(vm.Ctx.ChainID)
		peerSharedMemory := m.NewSharedMemory(avmID)

		// #nosec G404
		utxo := &avax.UTXO{
			UTXOID: avax.UTXOID{
				TxID:        ids.GenerateTestID(),
				OutputIndex: rand.Uint32(),
			},
			Asset: avax.Asset{ID: avaxAssetID},
			Out: &secp256k1fx.TransferOutput{
				Amt: amt,
				OutputOwners: secp256k1fx.OutputOwners{
					Locktime:  0,
					Addrs:     []ids.ShortID{recipientKey.PublicKey().Address()},
					Threshold: 1,
				},
			},
		}
		utxoBytes, err := Codec.Marshal(utxo)
		if err != nil {
			panic(err)
		}

		if err := peerSharedMemory.Put(vm.Ctx.ChainID, []*atomic.Element{{
			Key:   utxo.InputID().Bytes(),
			Value: utxoBytes,
			Traits: [][]byte{
				recipientKey.PublicKey().Address().Bytes(),
			},
		}}); err != nil {
			panic(err)
		}

		return sm
	}

	tests := []test{
		{
			description:   "recipient key can't pay fee;",
			sharedMemory:  fundedSharedMemory(vm.txFee - 1),
			recipientKeys: []*crypto.PrivateKeySECP256K1R{recipientKey},
			shouldErr:     true,
		},
		{

			description:   "recipient key pays fee",
			sharedMemory:  fundedSharedMemory(vm.txFee),
			recipientKeys: []*crypto.PrivateKeySECP256K1R{recipientKey},
			shouldErr:     false,
		},
	}

	vdb := versiondb.New(vm.DB)
	to := ids.GenerateTestShortID()
	for _, tt := range tests {
		vm.Ctx.SharedMemory = tt.sharedMemory
		tx, err := vm.newImportTx(avmID, to, tt.recipientKeys, ids.ShortEmpty)
		if err != nil {
			if !tt.shouldErr {
				t.Fatalf("test '%s' unexpectedly errored with: %s", tt.description, err)
			}
			continue
		} else if tt.shouldErr {
			t.Fatalf("test '%s' didn't error but it should have", tt.description)
		}
		unsignedTx := tx.UnsignedTx.(*UnsignedImportTx)
		if len(unsignedTx.ImportedInputs) == 0 {
			t.Fatalf("in test '%s', tx has no imported inputs", tt.description)
		} else if len(tx.Creds) != len(unsignedTx.Ins)+len(unsignedTx.ImportedInputs) {
			t.Fatalf("in test '%s', should have same number of credentials as inputs", tt.description)
		}
		totalIn := uint64(0)
		for _, in := range unsignedTx.Ins {
			totalIn += in.Input().Amount()
		}
		for _, in := range unsignedTx.ImportedInputs {
			totalIn += in.Input().Amount()
		}
		totalOut := uint64(0)
		for _, out := range unsignedTx.Outs {
			totalOut += out.Out.Amount()
		}
		if totalIn-totalOut != vm.txFee {
			t.Fatalf("in test '%s'. inputs (%d) != outputs (%d) + txFee (%d)", tt.description, totalIn, totalOut, vm.txFee)
		}
		vdb.Abort()
	}
}
