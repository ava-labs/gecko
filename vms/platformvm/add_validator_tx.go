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
	errNilTx                     = errors.New("tx is nil")
	errWeightTooSmall            = errors.New("weight of this validator is too low")
	errWeightTooLarge            = errors.New("weight of this validator is too large")
	errStakeTooShort             = errors.New("staking period is too short")
	errStakeTooLong              = errors.New("staking period is too long")
	errInsufficientDelegationFee = errors.New("staker charges an insufficient delegation fee")
	errTooManyShares             = fmt.Errorf("a staker can only require at most %d shares from delegators", PercentDenominator)

	_ UnsignedProposalTx = &UnsignedAddValidatorTx{}
	_ TimedTx            = &UnsignedAddValidatorTx{}
)

// UnsignedAddValidatorTx is an unsigned addValidatorTx
type UnsignedAddValidatorTx struct {
	// Metadata, inputs and outputs
	BaseTx `serialize:"true"`
	// Describes the delegatee
	Validator Validator `serialize:"true" json:"validator"`
	// Where to send staked tokens when done validating
	Stake []*avax.TransferableOutput `serialize:"true" json:"stake"`
	// Where to send staking rewards when done validating
	RewardsOwner verify.Verifiable `serialize:"true" json:"rewardsOwner"`
	// Fee this validator charges delegators as a percentage, times 10,000
	// For example, if this validator has Shares=300,000 then they take 30% of rewards from delegators
	Shares uint32 `serialize:"true" json:"shares"`
}

// StartTime of this validator
func (tx *UnsignedAddValidatorTx) StartTime() time.Time {
	return tx.Validator.StartTime()
}

// EndTime of this validator
func (tx *UnsignedAddValidatorTx) EndTime() time.Time {
	return tx.Validator.EndTime()
}

// Weight of this validator
func (tx *UnsignedAddValidatorTx) Weight() uint64 {
	return tx.Validator.Weight()
}

// Verify return nil iff [tx] is valid
func (tx *UnsignedAddValidatorTx) Verify(
	ctx *snow.Context,
	c codec.Manager,
	minStake uint64,
	maxStake uint64,
	minStakeDuration time.Duration,
	maxStakeDuration time.Duration,
	minDelegationFee uint32,
) error {
	switch {
	case tx == nil:
		return errNilTx
	case tx.syntacticallyVerified: // already passed syntactic verification
		return nil
	case tx.Validator.Wght < minStake: // Ensure validator is staking at least the minimum amount
		return errWeightTooSmall
	case tx.Validator.Wght > maxStake: // Ensure validator isn't staking too much
		return errWeightTooLarge
	case tx.Shares > PercentDenominator: // Ensure delegators shares are in the allowed amount
		return errTooManyShares
	case tx.Shares < minDelegationFee:
		return errInsufficientDelegationFee
	}

	duration := tx.Validator.Duration()
	switch {
	case duration < minStakeDuration: // Ensure staking length is not too short
		return errStakeTooShort
	case duration > maxStakeDuration: // Ensure staking length is not too long
		return errStakeTooLong
	}

	if err := tx.BaseTx.Verify(ctx, c); err != nil {
		return fmt.Errorf("failed to verify BaseTx: %w", err)
	}
	if err := verify.All(&tx.Validator, tx.RewardsOwner); err != nil {
		return fmt.Errorf("failed to verify validator or rewards owner: %w", err)
	}

	totalStakeWeight := uint64(0)
	for _, out := range tx.Stake {
		if err := out.Verify(); err != nil {
			return fmt.Errorf("failed to verify output: %w", err)
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
		return fmt.Errorf("validator weight %d is not equal to total stake weight %d", tx.Validator.Wght, totalStakeWeight)
	}

	// cache that this is valid
	tx.syntacticallyVerified = true
	return nil
}

// SemanticVerify this transaction is valid.
func (tx *UnsignedAddValidatorTx) SemanticVerify(
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
		vm.minValidatorStake,
		vm.maxValidatorStake,
		vm.minStakeDuration,
		vm.maxStakeDuration,
		vm.minDelegationFee,
	); err != nil {
		return nil, nil, nil, nil, permError{err}
	}

	outs := make([]*avax.TransferableOutput, len(tx.Outs)+len(tx.Stake))
	copy(outs, tx.Outs)
	copy(outs[len(tx.Outs):], tx.Stake)

	if vm.bootstrapped {
		// Ensure the proposed validator starts after the current time
		if currentTime, err := vm.getTimestamp(db); err != nil {
			return nil, nil, nil, nil, tempError{
				fmt.Errorf("failed to get timestamp: %w", err),
			}
		} else if startTime := tx.StartTime(); !currentTime.Before(startTime) {
			return nil, nil, nil, nil, permError{
				fmt.Errorf("validator's start time (%s) at or before current timestamp (%s)",
					startTime,
					currentTime,
				),
			}
		} else if startTime.After(currentTime.Add(maxFutureStartTime)) {
			return nil, nil, nil, nil, permError{
				fmt.Errorf(
					"validator start time (%s) more than two weeks after current chain timestamp (%s)",
					startTime,
					currentTime,
				),
			}
		}

		_, isValidator, err := vm.isValidator(db, constants.PrimaryNetworkID, tx.Validator.NodeID)
		if err != nil {
			return nil, nil, nil, nil, tempError{
				fmt.Errorf(
					"failed to get whether %s is a validator: %w",
					tx.Validator.NodeID,
					err,
				),
			}
		}
		if isValidator {
			return nil, nil, nil, nil, permError{
				fmt.Errorf(
					"validator %s is already a primary network validator",
					tx.Validator.NodeID,
				),
			}
		}

		// Ensure that the period this validator validates the specified subnet
		// is a subnet of the time they will validate the primary network.
		_, willBeValidator, err := vm.willBeValidator(db, constants.PrimaryNetworkID, tx.Validator.NodeID)
		if err != nil {
			return nil, nil, nil, nil, tempError{
				fmt.Errorf(
					"failed to get whether %s will be a validator: %w",
					tx.Validator.NodeID,
					err,
				),
			}
		}
		if willBeValidator {
			return nil, nil, nil, nil, permError{
				fmt.Errorf(
					"validator %s is already a primary network validator",
					tx.Validator.NodeID,
				),
			}
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

	// Verify inputs/outputs and update the UTXO set
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

	// Add validator to set of pending validators
	if err := vm.enqueueStaker(onCommitDB, constants.PrimaryNetworkID, stx); err != nil {
		return nil, nil, nil, nil, tempError{
			fmt.Errorf("failed to enqueue staker: %w", err),
		}
	}

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
func (tx *UnsignedAddValidatorTx) InitiallyPrefersCommit(vm *VM) bool {
	return tx.StartTime().After(vm.clock.Time())
}

// NewAddValidatorTx returns a new NewAddValidatorTx
func (vm *VM) newAddValidatorTx(
	stakeAmt, // Amount the delegator stakes
	startTime, // Unix time they start delegating
	endTime uint64, // Unix time they stop delegating
	nodeID ids.ShortID, // ID of the node we are delegating to
	rewardAddress ids.ShortID, // Address to send reward to, if applicable
	shares uint32, // 10,000 times percentage of reward taken from delegators
	keys []*crypto.PrivateKeySECP256K1R, // Keys providing the staked tokens
	changeAddr ids.ShortID, // Address to send change to, if there is any
) (*Tx, error) {
	ins, unlockedOuts, lockedOuts, signers, err := vm.stake(vm.DB, keys, stakeAmt, 0, changeAddr)
	if err != nil {
		return nil, fmt.Errorf("couldn't generate tx inputs/outputs: %w", err)
	}
	// Create the tx
	utx := &UnsignedAddValidatorTx{
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
		Shares: shares,
	}
	tx := &Tx{UnsignedTx: utx}
	if err := tx.Sign(vm.codec, signers); err != nil {
		return nil, err
	}
	return tx, utx.Verify(
		vm.Ctx,
		vm.codec,
		vm.minValidatorStake,
		vm.maxValidatorStake,
		vm.minStakeDuration,
		vm.maxStakeDuration,
		vm.minDelegationFee,
	)
}
