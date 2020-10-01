// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"reflect"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/utils/hashing"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"
)

func TestAddSubnetValidatorTxSyntacticVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	nodeID := keys[0].PublicKey().Address()

	// Case: tx is nil
	var unsignedTx *UnsignedAddSubnetValidatorTx
	if err := unsignedTx.Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because tx is nil")
	}

	// Case: Wrong network ID
	tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).NetworkID++
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because the wrong network ID was used")
	}

	// Case: Missing Node ID
	tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Validator.NodeID = ids.ShortID{ID: nil}
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because NodeID is empty")
	}

	// Case: Missing Subnet ID
	tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Validator.Subnet = ids.ID{ID: nil}
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because Subnet ID is nil")
	}

	// Case: No weight
	tx, err = vm.newAddSubnetValidatorTx(
		1,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Validator.Wght = 0
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because of no weight")
	}

	// Case: Subnet auth indices not unique
	tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix())-1,
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).SubnetAuth.(*secp256k1fx.Input).SigIndices[0] =
		tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).SubnetAuth.(*secp256k1fx.Input).SigIndices[1]
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err = tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because sig indices weren't unique")
	}

	// Case: Validation length is too short
	tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateStartTime.Add(defaultMinStakingDuration).Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Validator.End-- // 1 less than min duration
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because validation length too short")
	}

	// Case: Validation length is too long
	tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateStartTime.Add(defaultMaxStakingDuration).Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Validator.End++ // 1 more than max duration
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because validation length too long")
	}

	// Case: Valid
	if tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if err := tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err != nil {
		t.Fatal(err)
	}
}

func TestAddSubnetValidatorTxSemanticVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	nodeID := keys[0].PublicKey().Address()

	// Case: Proposed validator currently validating primary network
	// but stops validating subnet after stops validating primary network
	// (note that keys[0] is a genesis validator)
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix())+1,
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed because validator stops validating primary network earlier than subnet")
	}

	// Case: Proposed validator currently validating primary network
	// and proposed subnet validation period is subset of
	// primary network validation period
	// (note that keys[0] is a genesis validator)
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()+1),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err != nil {
		t.Fatal(err)
	}

	// Add a validator to pending validator set of primary network
	key, err := vm.factory.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pendingDSValidatorID := key.PublicKey().Address()

	// starts validating primary network 10 seconds after genesis
	DSStartTime := defaultGenesisTime.Add(10 * time.Second)
	DSEndTime := DSStartTime.Add(5 * defaultMinStakingDuration)

	addDSTx, err := vm.newAddValidatorTx(
		vm.minValidatorStake,                    // stake amount
		uint64(DSStartTime.Unix()),              // start time
		uint64(DSEndTime.Unix()),                // end time
		pendingDSValidatorID,                    // node ID
		nodeID,                                  // reward address
		PercentDenominator,                      // shares
		[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
		ids.ShortEmpty,                          // change addr

	)
	if err != nil {
		t.Fatal(err)
	}

	// Case: Proposed validator isn't in pending or current validator sets
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(DSStartTime.Unix()), // start validating subnet before primary network
		uint64(DSEndTime.Unix()),
		pendingDSValidatorID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed because validator not in the current or pending validator sets of the primary network")
	}

	if err := vm.addStaker(vm.DB, constants.PrimaryNetworkID, &rewardTx{
		Reward: 0,
		Tx:     *addDSTx,
	}); err != nil {
		t.Fatal(err)
	}
	// Node with ID key.PublicKey().Address() now a pending validator for primary network

	// Case: Proposed validator is pending validator of primary network
	// but starts validating subnet before primary network
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(DSStartTime.Unix())-1, // start validating subnet before primary network
		uint64(DSEndTime.Unix()),
		pendingDSValidatorID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed because validator starts validating primary " +
			"network before starting to validate primary network")
	}

	// Case: Proposed validator is pending validator of primary network
	// but stops validating subnet after primary network
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(DSStartTime.Unix()),
		uint64(DSEndTime.Unix())+1, // stop validating subnet after stopping validating primary network
		pendingDSValidatorID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed because validator stops validating primary " +
			"network after stops validating primary network")
	}

	// Case: Proposed validator is pending validator of primary network
	// and period validating subnet is subset of time validating primary network
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(DSStartTime.Unix()), // same start time as for primary network
		uint64(DSEndTime.Unix()),   // same end time as for primary network
		pendingDSValidatorID,
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err != nil {
		t.Fatalf("should have passed verification")
	}

	// Case: Proposed validator start validating at/before current timestamp
	// First, advance the timestamp
	newTimestamp := defaultGenesisTime.Add(2 * time.Second)
	if err := vm.putTimestamp(vm.DB, newTimestamp); err != nil {
		t.Fatal(err)
	}

	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,               // weight
		uint64(newTimestamp.Unix()), // start time
		uint64(newTimestamp.Add(defaultMinStakingDuration).Unix()), // end time
		nodeID,           // node ID
		testSubnet1.ID(), // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because starts validating at current timestamp")
	}

	// reset the timestamp
	if err := vm.putTimestamp(vm.DB, defaultGenesisTime); err != nil {
		t.Fatal(err)
	}

	// Case: Proposed validator already validating the subnet
	// First, add validator as validator of subnet
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,                           // weight
		uint64(defaultValidateStartTime.Unix()), // start time
		uint64(defaultValidateEndTime.Unix()),   // end time
		nodeID,                                  // node ID
		testSubnet1.ID(),                        // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if err := vm.addStaker(vm.DB, testSubnet1.ID(), &rewardTx{
		Reward: 0,
		Tx:     *tx,
	}); err != nil {
		t.Fatal(err)
	}
	// Node with ID nodeIDKey.PublicKey().Address() now validating subnet with ID testSubnet1.ID

	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,                           // weight
		uint64(defaultValidateStartTime.Unix()), // start time
		uint64(defaultValidateEndTime.Unix()),   // end time
		nodeID,                                  // node ID
		testSubnet1.ID(),                        // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because validator already validating the specified subnet")
	} else if err := vm.removeStaker(vm.DB, testSubnet1.ID(), &rewardTx{
		Reward: 0,
		Tx:     *tx,
	}); err != nil {
		t.Fatal(err)
	}

	// Case: Too many signatures
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,                     // weight
		uint64(defaultGenesisTime.Unix()), // start time
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix())+1, // end time
		nodeID,           // node ID
		testSubnet1.ID(), // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1], testSubnet1ControlKeys[2]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because tx has 3 signatures but only 2 needed")
	}

	// Case: Too few signatures
	tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,                     // weight
		uint64(defaultGenesisTime.Unix()), // start time
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()), // end time
		nodeID,           // node ID
		testSubnet1.ID(), // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[2]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	// Remove a signature
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).SubnetAuth.(*secp256k1fx.Input).SigIndices =
		tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).SubnetAuth.(*secp256k1fx.Input).SigIndices[1:]
		// This tx was syntactically verified when it was created...pretend it wan't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).syntacticallyVerified = false
	if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because not enough control sigs")
	}

	// Case: Control Signature from invalid key (keys[3] is not a control key)
	tx, err = vm.newAddSubnetValidatorTx(
		defaultWeight,                     // weight
		uint64(defaultGenesisTime.Unix()), // start time
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix()), // end time
		nodeID,           // node ID
		testSubnet1.ID(), // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], keys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	// Replace a valid signature with one from keys[3]
	sig, err := keys[3].SignHash(hashing.ComputeHash256(tx.UnsignedBytes()))
	if err != nil {
		t.Fatal(err)
	}
	copy(tx.Creds[0].(*secp256k1fx.Credential).Sigs[0][:], sig)
	if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because a control sig is invalid")
	}

	// Case: Proposed validator in pending validator set for subnet
	// First, add validator to pending validator set of subnet
	if tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,                       // weight
		uint64(defaultGenesisTime.Unix())+1, // start time
		uint64(defaultGenesisTime.Add(defaultMinStakingDuration).Unix())+1, // end time
		nodeID,           // node ID
		testSubnet1.ID(), // subnet ID
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if err = vm.addStaker(vm.DB, testSubnet1.ID(), &rewardTx{
		Reward: 0,
		Tx:     *tx,
	}); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should have failed verification because validator already in pending validator set of the specified subnet")
	}
}

// Test that marshalling/unmarshalling works
func TestAddSubnetValidatorMarshal(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	var unmarshaledTx Tx

	// valid tx
	tx, err := vm.newAddSubnetValidatorTx(
		defaultWeight,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		keys[0].PublicKey().Address(),
		testSubnet1.ID(),
		[]*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	} else if txBytes, err := Codec.Marshal(tx); err != nil {
		t.Fatal(err)
	} else if err := Codec.Unmarshal(txBytes, &unmarshaledTx); err != nil {
		t.Fatal(err)
	} else if err := unmarshaledTx.Sign(vm.codec, nil); err != nil {
		t.Fatal(err)
	} else if err := unmarshaledTx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.txFee,
		vm.Ctx.AVAXAssetID,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err != nil {
		t.Fatal(err)
	}
	if tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Memo == nil { // reflect.DeepEqual considers []byte{} and nil to be different so change nil to []byte{}
		tx.UnsignedTx.(*UnsignedAddSubnetValidatorTx).Memo = []byte{}
	}
	if !reflect.DeepEqual(*tx, unmarshaledTx) {
		t.Fatal("should be equal")
	}
}
