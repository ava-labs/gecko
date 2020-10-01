// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/vms/avm"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestUnsignedCreateChainTxVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	type test struct {
		description string
		shouldErr   bool
		subnetID    ids.ID
		genesisData []byte
		vmID        ids.ID
		fxIDs       []ids.ID
		chainName   string
		keys        []*crypto.PrivateKeySECP256K1R
		setup       func(*UnsignedCreateChainTx) *UnsignedCreateChainTx
	}

	tests := []test{
		{
			description: "tx is nil",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup:       func(*UnsignedCreateChainTx) *UnsignedCreateChainTx { return nil },
		},
		{
			description: "vm ID is empty",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup:       func(tx *UnsignedCreateChainTx) *UnsignedCreateChainTx { tx.VMID = ids.ID{ID: nil}; return tx },
		},
		{
			description: "subnet ID is empty",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup:       func(tx *UnsignedCreateChainTx) *UnsignedCreateChainTx { tx.SubnetID = ids.ID{ID: nil}; return tx },
		},
		{
			description: "subnet ID is platform chain's ID",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup:       func(tx *UnsignedCreateChainTx) *UnsignedCreateChainTx { tx.SubnetID = vm.Ctx.ChainID; return tx },
		},
		{
			description: "chain name is too long",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup: func(tx *UnsignedCreateChainTx) *UnsignedCreateChainTx {
				tx.ChainName = string(make([]byte, maxNameLen+1))
				return tx
			},
		},
		{
			description: "chain name has invalid character",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup: func(tx *UnsignedCreateChainTx) *UnsignedCreateChainTx {
				tx.ChainName = "⌘"
				return tx
			},
		},
		{
			description: "genesis data is too long",
			shouldErr:   true,
			subnetID:    testSubnet1.ID(),
			genesisData: nil,
			vmID:        avm.ID,
			fxIDs:       nil,
			chainName:   "yeet",
			keys:        []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
			setup: func(tx *UnsignedCreateChainTx) *UnsignedCreateChainTx {
				tx.GenesisData = make([]byte, maxGenesisLen+1)
				return tx
			},
		},
	}

	for _, test := range tests {
		tx, err := vm.newCreateChainTx(
			test.subnetID,
			test.genesisData,
			test.vmID,
			test.fxIDs,
			test.chainName,
			test.keys,
			ids.ShortEmpty, // change addr
		)
		if err != nil {
			t.Fatal(err)
		}
		tx.UnsignedTx.(*UnsignedCreateChainTx).syntacticallyVerified = false
		tx.UnsignedTx = test.setup(tx.UnsignedTx.(*UnsignedCreateChainTx))
		if err := tx.UnsignedTx.(*UnsignedCreateChainTx).Verify(vm.Ctx, vm.codec, vm.txFee, vm.Ctx.AVAXAssetID); err != nil && !test.shouldErr {
			t.Fatalf("test '%s' shouldn't have errored but got: %s", test.description, err)
		} else if err == nil && test.shouldErr {
			t.Fatalf("test '%s' didn't error but should have", test.description)
		}
	}
}

// Ensure SemanticVerify fails when there are not enough control sigs
func TestCreateChainTxInsufficientControlSigs(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	tx, err := vm.newCreateChainTx(
		testSubnet1.ID(),
		nil,
		avm.ID,
		nil,
		"chain name",
		[]*crypto.PrivateKeySECP256K1R{keys[0], keys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	// Remove a signature
	tx.Creds[0].(*secp256k1fx.Credential).Sigs = tx.Creds[0].(*secp256k1fx.Credential).Sigs[1:]
	if _, err := tx.UnsignedTx.(UnsignedDecisionTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have errored because a sig is missing")
	}
}

// Ensure SemanticVerify fails when an incorrect control signature is given
func TestCreateChainTxWrongControlSig(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	tx, err := vm.newCreateChainTx( // create a tx
		testSubnet1.ID(),
		nil,
		avm.ID,
		nil,
		"chain name",
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	// Generate new, random key to sign tx with
	factory := crypto.FactorySECP256K1R{}
	key, err := factory.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	// Replace a valid signature with one from another key
	sig, err := key.SignHash(hashing.ComputeHash256(tx.UnsignedBytes()))
	if err != nil {
		t.Fatal(err)
	}
	copy(tx.Creds[0].(*secp256k1fx.Credential).Sigs[0][:], sig)
	if _, err = tx.UnsignedTx.(UnsignedDecisionTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because a sig is invalid")
	}
}

// Ensure SemanticVerify fails when the Subnet the blockchain specifies as
// its validator set doesn't exist
func TestCreateChainTxNoSuchSubnet(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	tx, err := vm.newCreateChainTx(
		testSubnet1.ID(),
		nil,
		avm.ID,
		nil,
		"chain name",
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedCreateChainTx).SubnetID = ids.GenerateTestID()
	if _, err := tx.UnsignedTx.(UnsignedDecisionTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed because subent doesn't exist")
	}
}

func TestCreateChainTxAlreadyExists(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	// create a tx
	tx, err := vm.newCreateChainTx(
		testSubnet1.ID(),
		nil,
		avm.ID,
		nil,
		"chain name",
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	// put the chain in existing chain list
	if err := vm.putChains(vm.DB, []*Tx{tx}); err != nil {
		t.Fatal(err)
	}

	_, err = tx.UnsignedTx.(UnsignedDecisionTx).SemanticVerify(vm, vm.DB, tx)
	if err == nil {
		t.Fatalf("should have failed because the chain already exists")
	}
}

// Ensure valid tx passes semanticVerify
func TestCreateChainTxValid(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	// create a valid tx
	tx, err := vm.newCreateChainTx(
		testSubnet1.ID(),
		nil,
		avm.ID,
		nil,
		"chain name",
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tx.UnsignedTx.(UnsignedDecisionTx).SemanticVerify(vm, vm.DB, tx)
	if err != nil {
		t.Fatalf("expected tx to pass verification but got error: %v", err)
	}
}
