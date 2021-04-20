// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"fmt"

	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

// Parameters the avalanche parameters include the snowball parameters and the
// optimal number of parents
type Parameters struct {
	snowball.Parameters
	Parents, BatchSize int
}

// Valid returns nil if the parameters describe a valid initialization.
func (p Parameters) Valid() error {
	switch {
	case p.Parents <= 1:
		return fmt.Errorf("parents = %d: Fails the condition that: 1 < Parents", p.Parents)
	case p.BatchSize <= 0:
		return fmt.Errorf("batchSize = %d: Fails the condition that: 0 < BatchSize", p.BatchSize)
	default:
		return p.Parameters.Verify()
	}
}
