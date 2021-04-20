// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"testing"

	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

func TestParametersValid(t *testing.T) {
	p := Parameters{
		Parameters: snowball.Parameters{
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}

	if err := p.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestParametersInvalidParents(t *testing.T) {
	p := Parameters{
		Parameters: snowball.Parameters{
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   1,
		BatchSize: 1,
	}

	if err := p.Valid(); err == nil {
		t.Fatalf("Should have failed due to invalid parents")
	}
}

func TestParametersInvalidBatchSize(t *testing.T) {
	p := Parameters{
		Parameters: snowball.Parameters{
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 0,
	}

	if err := p.Valid(); err == nil {
		t.Fatalf("Should have failed due to invalid batch size")
	}
}
