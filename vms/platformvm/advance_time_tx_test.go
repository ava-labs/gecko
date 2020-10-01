// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
)

// Ensure semantic verification fails when proposed timestamp is at or before current timestamp
func TestAdvanceTimeTxTimestampTooEarly(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	if tx, err := vm.newAdvanceTimeTx(defaultGenesisTime); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should've failed verification because proposed timestamp same as current timestamp")
	}
}

// Ensure semantic verification fails when proposed timestamp is after next validator set change time
func TestAdvanceTimeTxTimestampTooLate(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()

	// Case: Timestamp is after next validator start time
	// Add a pending validator
	pendingValidatorStartTime := defaultGenesisTime.Add(1 * time.Second)
	pendingValidatorEndTime := pendingValidatorStartTime.Add(defaultMinStakingDuration)
	nodeIDKey, _ := vm.factory.NewPrivateKey()
	nodeID := nodeIDKey.PublicKey().Address()
	addPendingValidatorTx, err := vm.newAddValidatorTx(
		vm.minValidatorStake,
		uint64(pendingValidatorStartTime.Unix()),
		uint64(pendingValidatorEndTime.Unix()),
		nodeID,
		nodeID,
		PercentDenominator,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := vm.enqueueStaker(vm.DB, constants.PrimaryNetworkID, addPendingValidatorTx); err != nil {
		t.Fatal(err)
	}

	tx, err := vm.newAdvanceTimeTx(pendingValidatorStartTime.Add(1 * time.Second))
	if err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should've failed verification because proposed timestamp is after pending validator start time")
	}
	vm.Shutdown()
	vm.Ctx.Lock.Unlock()

	// Case: Timestamp is after next validator end time
	vm, _ = defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	// fast forward clock to 10 seconds before genesis validators stop validating
	vm.clock.Set(defaultValidateEndTime.Add(-10 * time.Second))

	// Proposes advancing timestamp to 1 second after genesis validators stop validating
	if tx, err := vm.newAdvanceTimeTx(defaultValidateEndTime.Add(1 * time.Second)); err != nil {
		t.Fatal(err)
	} else if _, _, _, _, err = tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx); err == nil {
		t.Fatal("should've failed verification because proposed timestamp is after pending validator start time")
	}
}

// Ensure semantic verification updates the current and pending validator sets correctly
func TestAdvanceTimeTxUpdateValidators(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	// Case: Timestamp is after next validator start time
	// Add a pending validator
	pendingValidatorStartTime := defaultGenesisTime.Add(1 * time.Second)
	pendingValidatorEndTime := pendingValidatorStartTime.Add(defaultMinStakingDuration)
	nodeIDKey, _ := vm.factory.NewPrivateKey()
	nodeID := nodeIDKey.PublicKey().Address()
	addPendingValidatorTx, err := vm.newAddValidatorTx(
		vm.minValidatorStake,
		uint64(pendingValidatorStartTime.Unix()),
		uint64(pendingValidatorEndTime.Unix()),
		nodeID,
		nodeID,
		PercentDenominator,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := vm.enqueueStaker(vm.DB, constants.PrimaryNetworkID, addPendingValidatorTx); err != nil {
		t.Fatal(err)
	}

	tx, err := vm.newAdvanceTimeTx(pendingValidatorStartTime)
	if err != nil {
		t.Fatal(err)
	}
	onCommit, onAbort, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vm.DB, tx)
	if err != nil {
		t.Fatal(err)
	}

	if validatorTx, isValidator, err := vm.isValidator(onCommit, constants.PrimaryNetworkID, nodeID); err != nil {
		t.Fatal(err)
	} else if !isValidator {
		t.Fatalf("Should have added the validator to the validator set")
	} else if !validatorTx.ID().Equals(addPendingValidatorTx.ID()) {
		t.Fatalf("Added the wrong tx to the validator set")
	} else if _, willBeValidator, err := vm.willBeValidator(onCommit, constants.PrimaryNetworkID, nodeID); err != nil {
		t.Fatal(err)
	} else if willBeValidator {
		t.Fatalf("Should have removed the validator from the pending validator set")
	} else if tx, err := vm.nextStakerStop(onCommit, constants.PrimaryNetworkID); err != nil {
		t.Fatal(err)
	} else if tx.Reward != 1370 { // See rewards tests
		t.Fatalf("Expected reward of %d but was %d", 1370, tx.Reward)
	}

	if _, isValidator, err := vm.isValidator(onAbort, constants.PrimaryNetworkID, nodeID); err != nil {
		t.Fatal(err)
	} else if isValidator {
		t.Fatalf("Shouldn't have added the validator to the validator set")
	} else if validatorTx, willBeValidator, err := vm.willBeValidator(onAbort, constants.PrimaryNetworkID, nodeID); err != nil {
		t.Fatal(err)
	} else if !willBeValidator {
		t.Fatalf("Shouldn't have removed the validator from the pending validator set")
	} else if !validatorTx.ID().Equals(addPendingValidatorTx.ID()) {
		t.Fatalf("Added the wrong tx to the pending validator set")
	}
}

// Test method InitiallyPrefersCommit
func TestAdvanceTimeTxInitiallyPrefersCommit(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	// Proposed advancing timestamp to 1 second after current timestamp
	tx, err := vm.newAdvanceTimeTx(defaultGenesisTime.Add(1 * time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if tx.UnsignedTx.(UnsignedProposalTx).InitiallyPrefersCommit(vm) {
		t.Fatal("should not prefer to commit this tx because its proposed timestamp is after wall clock time")
	}

	// advance wall clock time
	vm.clock.Set(defaultGenesisTime.Add(2 * time.Second))
	if !tx.UnsignedTx.(UnsignedProposalTx).InitiallyPrefersCommit(vm) {
		t.Fatal("should prefer to commit this tx because its proposed timestamp is before wall clock time")
	}
}

// Ensure marshaling/unmarshaling works
func TestAdvanceTimeTxUnmarshal(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	tx, err := vm.newAdvanceTimeTx(defaultGenesisTime)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := Codec.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}

	var unmarshaledTx Tx
	if err := Codec.Unmarshal(bytes, &unmarshaledTx); err != nil {
		t.Fatal(err)
	} else if tx.UnsignedTx.(*UnsignedAdvanceTimeTx).Time != unmarshaledTx.UnsignedTx.(*UnsignedAdvanceTimeTx).Time {
		t.Fatal("should have same timestamp")
	}
}
