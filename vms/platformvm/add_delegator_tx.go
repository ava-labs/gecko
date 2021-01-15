// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"errors"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/codec"
	"github.com/ava-labs/avalanchego/database"
	"github.com/ava-labs/avalanchego/database/versiondb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/crypto"
	"github.com/ava-labs/avalanchego/vms/components/avax"
	"github.com/ava-labs/avalanchego/vms/components/verify"
	"github.com/ava-labs/avalanchego/vms/secp256k1fx"

	safemath "github.com/ava-labs/avalanchego/utils/math"
)

var (
	errDelegatorSubset = errors.New("delegator's time range must be a subset of the validator's time range")
	errInvalidState    = errors.New("generated output isn't valid state")
	errCapWeightBroken = errors.New("validator would surpass maximum weight")
	errOverDelegated   = errors.New("validator would be over delegated")

	_ UnsignedProposalTx = &UnsignedAddDelegatorTx{}
	_ TimedTx            = &UnsignedAddDelegatorTx{}
)

// UnsignedAddDelegatorTx is an unsigned addDelegatorTx
type UnsignedAddDelegatorTx struct {
	// Metadata, inputs and outputs
	BaseTx `serialize:"true"`
	// Describes the delegatee
	Validator Validator `serialize:"true" json:"validator"`
	// Where to send staked tokens when done validating
	Stake []*avax.TransferableOutput `serialize:"true" json:"stake"`
	// Where to send staking rewards when done validating
	RewardsOwner verify.Verifiable `serialize:"true" json:"rewardsOwner"`
}

// StartTime of this validator
func (tx *UnsignedAddDelegatorTx) StartTime() time.Time {
	return tx.Validator.StartTime()
}

// EndTime of this validator
func (tx *UnsignedAddDelegatorTx) EndTime() time.Time {
	return tx.Validator.EndTime()
}

// Weight of this validator
func (tx *UnsignedAddDelegatorTx) Weight() uint64 {
	return tx.Validator.Weight()
}

// Verify return nil iff [tx] is valid
func (tx *UnsignedAddDelegatorTx) Verify(
	ctx *snow.Context,
	c codec.Manager,
	minDelegatorStake uint64,
	minStakeDuration time.Duration,
	maxStakeDuration time.Duration,
) error {
	switch {
	case tx == nil:
		return errNilTx
	case tx.syntacticallyVerified: // already passed syntactic verification
		return nil
	}

	duration := tx.Validator.Duration()
	switch {
	case duration < minStakeDuration: // Ensure staking length is not too short
		return errStakeTooShort
	case duration > maxStakeDuration: // Ensure staking length is not too long
		return errStakeTooLong
	}

	if err := tx.BaseTx.Verify(ctx, c); err != nil {
		return err
	}
	if err := verify.All(&tx.Validator, tx.RewardsOwner); err != nil {
		return fmt.Errorf("failed to verify validator or rewards owner: %w", err)
	}

	totalStakeWeight := uint64(0)
	for _, out := range tx.Stake {
		if err := out.Verify(); err != nil {
			return fmt.Errorf("output verification failed: %w", err)
		}
		newWeight, err := safemath.Add64(totalStakeWeight, out.Output().Amount())
		if err != nil {
			return err
		}
		totalStakeWeight = newWeight
	}

	switch {
	case !avax.IsSortedTransferableOutputs(tx.Stake, c, codecVersion):
		return errOutputsNotSorted
	case totalStakeWeight != tx.Validator.Wght:
		return fmt.Errorf("delegator weight %d is not equal to total stake weight %d", tx.Validator.Wght, totalStakeWeight)
	case tx.Validator.Wght < minDelegatorStake:
		// Ensure validator is staking at least the minimum amount
		return errWeightTooSmall
	}

	// cache that this is valid
	tx.syntacticallyVerified = true
	return nil
}

// SemanticVerify this transaction is valid.
func (tx *UnsignedAddDelegatorTx) SemanticVerify(
	vm *VM,
	db database.Database,
	stx *Tx,
) (
	*versiondb.Database,
	*versiondb.Database,
	func() error,
	func() error,
	TxError,
) {
	// Verify the tx is well-formed
	if err := tx.Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		vm.minStakeDuration,
		vm.maxStakeDuration,
	); err != nil {
		return nil, nil, nil, nil, permError{err}
	}

	outs := make([]*avax.TransferableOutput, len(tx.Outs)+len(tx.Stake))
	copy(outs, tx.Outs)
	copy(outs[len(tx.Outs):], tx.Stake)

	if vm.bootstrapped {
		// Ensure the proposed validator starts after the current timestamp
		if currentTimestamp, err := vm.getTimestamp(db); err != nil {
			return nil, nil, nil, nil, tempError{
				fmt.Errorf("failed to get timestamp: %w", err),
			}
		} else if validatorStartTime := tx.StartTime(); !currentTimestamp.Before(validatorStartTime) {
			return nil, nil, nil, nil, permError{fmt.Errorf("chain timestamp (%s) not before validator's start time (%s)",
				currentTimestamp,
				validatorStartTime)}
		} else if validatorStartTime.After(currentTimestamp.Add(maxFutureStartTime)) {
			return nil, nil, nil, nil, permError{fmt.Errorf("validator start time (%s) more than two weeks after current chain timestamp (%s)", validatorStartTime, currentTimestamp)}
		}

		// Ensure that the period this delegator delegates is a subset of the time
		// the validator validates.
		vdr, isValidator, err := vm.isValidator(db, constants.PrimaryNetworkID, tx.Validator.NodeID)
		vdrWeight := uint64(0)
		if err != nil {
			return nil, nil, nil, nil, tempError{
				fmt.Errorf("failed to find whether %s is a validator: %w", tx.Validator.NodeID, err),
			}
		}
		if isValidator && !tx.Validator.BoundedBy(vdr.StartTime(), vdr.EndTime()) {
			return nil, nil, nil, nil, permError{errDelegatorSubset}
		}
		if !isValidator {
			// Ensure that the period this delegator delegates is a subset of the
			// time the validator will validates.
			vdr, willBeValidator, err := vm.willBeValidator(db, constants.PrimaryNetworkID, tx.Validator.NodeID)
			if err != nil {
				return nil, nil, nil, nil, tempError{
					fmt.Errorf("failed to find whether %s will be a validator: %w", tx.Validator.NodeID, err),
				}
			}
			if !willBeValidator || !tx.Validator.BoundedBy(vdr.StartTime(), vdr.EndTime()) {
				return nil, nil, nil, nil, permError{errDelegatorSubset}
			}
			vdrWeight = vdr.Weight()
		} else {
			vdrWeight = vdr.Weight()
		}

		maxWeight, err := vm.maxStakeAmount(db, constants.PrimaryNetworkID, tx.Validator.NodeID, tx.StartTime(), tx.EndTime())
		if err != nil {
			return nil, nil, nil, nil, tempError{err}
		}
		newWeight, err := safemath.Add64(maxWeight, tx.Validator.Wght)
		if err != nil {
			return nil, nil, nil, nil, permError{errStakeOverflow}
		}
		if newWeight > vm.maxValidatorStake {
			return nil, nil, nil, nil, permError{errCapWeightBroken}
		}

		delegationRestrict, err := safemath.Mul64(5, vdrWeight)
		if err != nil {
			return nil, nil, nil, nil, permError{errStakeOverflow}
		}
		if newWeight > delegationRestrict {
			return nil, nil, nil, nil, permError{errOverDelegated}
		}

		// Verify the flowcheck
		if err := vm.semanticVerifySpend(db, tx, tx.Ins, outs, stx.Creds, 0, vm.Ctx.AVAXAssetID); err != nil {
			switch err.(type) {
			case permError:
				return nil, nil, nil, nil, permError{
					fmt.Errorf("failed semanticVerifySpend: %w", err),
				}
			default:
				return nil, nil, nil, nil, tempError{
					fmt.Errorf("failed semanticVerifySpend: %w", err),
				}
			}
		}
	}

	txID := tx.ID()

	// Set up the DB if this tx is committed
	onCommitDB := versiondb.New(db)
	// Consume the UTXOS
	if err := vm.consumeInputs(onCommitDB, tx.Ins); err != nil {
		return nil, nil, nil, nil, tempError{
			fmt.Errorf("failed to consume inputs: %w", err),
		}
	}
	// Produce the UTXOS
	if err := vm.produceOutputs(onCommitDB, txID, tx.Outs); err != nil {
		return nil, nil, nil, nil, tempError{
			fmt.Errorf("failed to produce outputs: %w", err),
		}
	}

	// If this proposal is committed, update the pending validator set to include the delegator
	if err := vm.enqueueStaker(onCommitDB, constants.PrimaryNetworkID, stx); err != nil {
		return nil, nil, nil, nil, tempError{
			fmt.Errorf("failed to enqueue staker: %w", err),
		}
	}

	// Set up the DB if this tx is aborted
	onAbortDB := versiondb.New(db)
	// Consume the UTXOS
	if err := vm.consumeInputs(onAbortDB, tx.Ins); err != nil {
		return nil, nil, nil, nil, tempError{
			fmt.Errorf("failed to consume inputs: %w", err),
		}
	}
	// Produce the UTXOS
	if err := vm.produceOutputs(onAbortDB, txID, outs); err != nil {
		return nil, nil, nil, nil, tempError{
			fmt.Errorf("failed to produce outputs: %w", err),
		}
	}

	return onCommitDB, onAbortDB, nil, nil, nil
}

// InitiallyPrefersCommit returns true if the proposed validators start time is
// after the current wall clock time,
func (tx *UnsignedAddDelegatorTx) InitiallyPrefersCommit(vm *VM) bool {
	return tx.StartTime().After(vm.clock.Time())
}

// Creates a new transaction
func (vm *VM) newAddDelegatorTx(
	stakeAmt, // Amount the delegator stakes
	startTime, // Unix time they start delegating
	endTime uint64, // Unix time they stop delegating
	nodeID ids.ShortID, // ID of the node we are delegating to
	rewardAddress ids.ShortID, // Address to send reward to, if applicable
	keys []*crypto.PrivateKeySECP256K1R, // Keys providing the staked tokens
	changeAddr ids.ShortID, // Address to send change to, if there is any
) (*Tx, error) {
	ins, unlockedOuts, lockedOuts, signers, err := vm.stake(vm.DB, keys, stakeAmt, 0, changeAddr)
	if err != nil {
		return nil, fmt.Errorf("couldn't generate tx inputs/outputs: %w", err)
	}
	// Create the tx
	utx := &UnsignedAddDelegatorTx{
		BaseTx: BaseTx{BaseTx: avax.BaseTx{
			NetworkID:    vm.Ctx.NetworkID,
			BlockchainID: vm.Ctx.ChainID,
			Ins:          ins,
			Outs:         unlockedOuts,
		}},
		Validator: Validator{
			NodeID: nodeID,
			Start:  startTime,
			End:    endTime,
			Wght:   stakeAmt,
		},
		Stake: lockedOuts,
		RewardsOwner: &secp256k1fx.OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs:     []ids.ShortID{rewardAddress},
		},
	}
	tx := &Tx{UnsignedTx: utx}
	if err := tx.Sign(vm.codec, signers); err != nil {
		return nil, err
	}
	return tx, utx.Verify(
		vm.Ctx,
		vm.codec,
		vm.minDelegatorStake,
		vm.minStakeDuration,
		vm.maxStakeDuration,
	)
}
