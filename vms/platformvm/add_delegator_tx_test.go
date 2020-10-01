// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
)

func TestAddDelegatorTxSyntacticVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()

	nodeID := keys[0].PublicKey().Address()
	rewardAddress := nodeID

	// Case : tx is nil
	var unsignedTx *UnsignedAddDelegatorTx
	if err := unsignedTx.Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because tx is nil")
	}

	// Case: Wrong network ID
	tx, err := vm.newAddDelegatorTx(
		vm.minDelegatorStake,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		rewardAddress,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).NetworkID++
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddDelegatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because the wrong network ID was used")
	}

	// Case: Missing Node ID
	tx, err = vm.newAddDelegatorTx(
		vm.minDelegatorStake,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		rewardAddress,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).Validator.NodeID = ids.ShortID{}
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddDelegatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because NodeID is nil")
	}

	// Case: Not enough weight
	tx, err = vm.newAddDelegatorTx(
		vm.minDelegatorStake,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		rewardAddress,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).Validator.Wght = vm.minDelegatorStake - 1
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddDelegatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because of not enough weight")
	}

	// Case: Validation length is too short
	tx, err = vm.newAddDelegatorTx(
		vm.minDelegatorStake,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateStartTime.Add(defaultMinStakingDuration).Unix()),
		nodeID,
		rewardAddress,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	)
	if err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).Validator.End-- // 1 shorter than minimum stake time
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).syntacticallyVerified = false
	if err = tx.UnsignedTx.(*UnsignedAddDelegatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because validation length too short")
	}

	// Case: Validation length is too long
	if tx, err = vm.newAddDelegatorTx(
		vm.minDelegatorStake,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateStartTime.Add(defaultMaxStakingDuration).Unix()),
		nodeID,
		rewardAddress,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	}
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).Validator.End++ // 1 longer than maximum stake time
	// This tx was syntactically verified when it was created...pretend it wasn't so we don't use cache
	tx.UnsignedTx.(*UnsignedAddDelegatorTx).syntacticallyVerified = false
	if err := tx.UnsignedTx.(*UnsignedAddDelegatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err == nil {
		t.Fatal("should have errored because validation length too long")
	}

	// Case: Valid
	if tx, err = vm.newAddDelegatorTx(
		vm.minDelegatorStake,
		uint64(defaultValidateStartTime.Unix()),
		uint64(defaultValidateEndTime.Unix()),
		nodeID,
		rewardAddress,
		[]*crypto.PrivateKeySECP256K1R{keys[0]},
		ids.ShortEmpty, // change addr
	); err != nil {
		t.Fatal(err)
	} else if err := tx.UnsignedTx.(*UnsignedAddDelegatorTx).Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		defaultMinStakingDuration,
		defaultMaxStakingDuration,
	); err != nil {
		t.Fatal(err)
	}
}

func TestAddDelegatorTxSemanticVerify(t *testing.T) {
	vm, _ := defaultVM(t)
	vm.Ctx.Lock.Lock()
	defer func() {
		vm.Shutdown()
		vm.Ctx.Lock.Unlock()
	}()
	nodeID := keys[0].PublicKey().Address()
	rewardAddress := nodeID
	vdb := versiondb.New(vm.DB) // so tests don't interfere with one another
	currentTimestamp, err := vm.getTimestamp(vm.DB)
	if err != nil {
		t.Fatal(err)
	}

	keyIntf, err := vm.factory.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	newValidatorKey := keyIntf.(*crypto.PrivateKeySECP256K1R)
	newValidatorID := newValidatorKey.PublicKey().Address()
	newValidatorStartTime := uint64(defaultValidateStartTime.Add(5 * time.Second).Unix())
	newValidatorEndTime := uint64(defaultValidateEndTime.Add(-5 * time.Second).Unix())
	// [addValidator] adds a new validator to the primary network's pending validator set
	addValidator := func(db database.Database) {
		if tx, err := vm.newAddValidatorTx(
			vm.minValidatorStake,                    // stake amount
			newValidatorStartTime,                   // start time
			newValidatorEndTime,                     // end time
			newValidatorID,                          // node ID
			rewardAddress,                           // Reward Address
			PercentDenominator,                      // subnet
			[]*crypto.PrivateKeySECP256K1R{keys[0]}, // key
			ids.ShortEmpty,                          // change addr
		); err != nil {
			t.Fatal(err)
		} else if err := vm.addStaker(db, constants.PrimaryNetworkID, &rewardTx{
			Reward: 0,
			Tx:     *tx,
		}); err != nil {
			t.Fatal(err)
		}
	}

	type test struct {
		stakeAmount   uint64
		startTime     uint64
		endTime       uint64
		nodeID        ids.ShortID
		rewardAddress ids.ShortID
		feeKeys       []*crypto.PrivateKeySECP256K1R
		setup         func(db database.Database)
		shouldErr     bool
		description   string
	}

	tests := []test{
		{
			vm.minDelegatorStake,
			uint64(defaultValidateStartTime.Unix()),
			uint64(defaultValidateEndTime.Unix()) + 1,
			nodeID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			nil,
			true,
			"validator stops validating primary network earlier than subnet",
		},
		{
			vm.minDelegatorStake,
			uint64(currentTimestamp.Add(maxFutureStartTime + time.Second).Unix()),
			uint64(currentTimestamp.Add(maxFutureStartTime * 2).Unix()),
			nodeID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			nil,
			true,
			fmt.Sprintf("validator should not be added more than (%s) in the future", maxFutureStartTime),
		},
		{
			vm.minDelegatorStake,
			uint64(defaultValidateStartTime.Unix()),
			uint64(defaultValidateEndTime.Unix()) + 1,
			nodeID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			nil,
			true,
			"end time is after the primary network end time",
		},
		{
			vm.minDelegatorStake,
			uint64(defaultValidateStartTime.Add(5 * time.Second).Unix()),
			uint64(defaultValidateEndTime.Add(-5 * time.Second).Unix()),
			newValidatorID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			nil,
			true,
			"validator not in the current or pending validator sets of the subnet",
		},
		{
			vm.minDelegatorStake,
			newValidatorStartTime - 1, // start validating subnet before primary network
			newValidatorEndTime,
			newValidatorID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			addValidator,
			true,
			"validator starts validating subnet before primary network",
		},
		{
			vm.minDelegatorStake,
			newValidatorStartTime,
			newValidatorEndTime + 1, // stop validating subnet after stopping validating primary network
			newValidatorID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			addValidator,
			true,
			"validator stops validating primary network before subnet",
		},
		{
			vm.minDelegatorStake,
			newValidatorStartTime, // same start time as for primary network
			newValidatorEndTime,   // same end time as for primary network
			newValidatorID,
			rewardAddress,
			[]*crypto.PrivateKeySECP256K1R{keys[0]},
			addValidator,
			false,
			"valid",
		},
		{
			vm.minDelegatorStake, // weight
			uint64(currentTimestamp.Unix()),
			uint64(defaultValidateEndTime.Unix()),
			nodeID,                                  // node ID
			rewardAddress,                           // Reward Address
			[]*crypto.PrivateKeySECP256K1R{keys[0]}, // tx fee payer
			nil,
			true,
			"starts validating at current timestamp",
		},
		{
			vm.minDelegatorStake,                    // weight
			uint64(defaultValidateStartTime.Unix()), // start time
			uint64(defaultValidateEndTime.Unix()),   // end time
			nodeID,                                  // node ID
			rewardAddress,                           // Reward Address
			[]*crypto.PrivateKeySECP256K1R{keys[1]}, // tx fee payer
			func(db database.Database) { // Remove all UTXOs owned by keys[1]
				utxoIDs, err := vm.getReferencingUTXOs(db, keys[1].PublicKey().Address().Bytes(), ids.Empty, math.MaxInt32)
				if err != nil {
					t.Fatal(err)
				}
				for _, utxoID := range utxoIDs.List() {
					if err := vm.removeUTXO(db, utxoID); err != nil {
						t.Fatal(err)
					}
				}
			},
			true,
			"tx fee paying key has no funds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			vdb.Abort()
			tx, err := vm.newAddDelegatorTx(
				tt.stakeAmount,
				tt.startTime,
				tt.endTime,
				tt.nodeID,
				tt.rewardAddress,
				tt.feeKeys,
				ids.ShortEmpty, // change addr
			)
			if err != nil {
				t.Fatalf("couldn't build tx: %s", err)
			}
			if tt.setup != nil {
				tt.setup(vdb)
			}
			if _, _, _, _, err := tx.UnsignedTx.(UnsignedProposalTx).SemanticVerify(vm, vdb, tx); err != nil && !tt.shouldErr {
				t.Fatalf("shouldn't have errored but got %s", err)
			} else if err == nil && tt.shouldErr {
				t.Fatalf("expected test to error but got none")
			}
		})
	}
}
