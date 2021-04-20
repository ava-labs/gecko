// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package genesis

import (
	"time"

	"github.com/ava-labs/avalanchego/utils/constants"
)

// Params ...
type Params struct {
	// Transaction fee
	TxFee uint64
	// Transaction fee for transactions that create new state
	CreationTxFee uint64
	// Staking uptime requirements
	UptimeRequirement float64
	// Minimum stake, in nAVAX, required to validate the primary network
	MinValidatorStake uint64
	// Maximum stake, in nAVAX, allowed to be placed on a single validator in
	// the primary network
	MaxValidatorStake uint64
	// Minimum stake, in nAVAX, that can be delegated on the primary network
	MinDelegatorStake uint64
	// Minimum delegation fee, in the range [0, 1000000], that can be charged
	// for delegation on the primary network.
	MinDelegationFee uint32
	// MinStakeDuration is the minimum amount of time a validator can validate
	// for in a single period.
	MinStakeDuration time.Duration
	// MaxStakeDuration is the maximum amount of time a validator can validate
	// for in a single period.
	MaxStakeDuration time.Duration
	// StakeMintingPeriod is the amount of time for a consumption period.
	StakeMintingPeriod time.Duration
	// EpochFirstTransition is the time that the transition from epoch 0 to 1
	// should occur.
	EpochFirstTransition time.Time
	// EpochDuration is the amount of time that an epoch runs for.
	EpochDuration time.Duration
}

// GetParams ...
func GetParams(networkID uint32) *Params {
	switch networkID {
	case constants.MainnetID:
		return &MainnetParams
	case constants.FujiID:
		return &FujiParams
	case constants.LocalID:
		return &LocalParams
	default:
		return &LocalParams
	}
}
