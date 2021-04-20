// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package platformvm

import (
	"fmt"

	"github.com/ava-labs/avalanchego/utils/constants"
)

// Health implements the common.VM interface
func (vm *VM) HealthCheck() (interface{}, error) {
	// Returns nil if this node is connected to > alpha percent of the Primary Network's stake
	percentConnected, err := vm.getPercentConnected()
	if err != nil {
		return nil, fmt.Errorf("couldn't get percent connected: %w", err)
	}

	vm.metrics.percentConnected.Set(percentConnected)

	details := map[string]float64{
		"percentConnected": percentConnected,
	}
	if percentConnected < constants.MinConnectedStake { // Use alpha from consensus instead of const
		return details, fmt.Errorf("connected to %f percent of the stake; should be connected to at least %f",
			percentConnected,
			constants.MinConnectedStake,
		)
	}
	return details, nil
}
