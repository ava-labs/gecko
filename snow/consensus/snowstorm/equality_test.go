// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowstorm

import (
	"math/rand"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	sbcon "github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

func TestConflictGraphEquality(t *testing.T) {
	Setup()

	numColors := 5
	colorsPerConsumer := 2
	maxInputConflicts := 2
	numNodes := 100
	params := sbcon.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     20,
		Alpha:                 11,
		BetaVirtuous:          20,
		BetaRogue:             30,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	seed := int64(0)

	nDirected := Network{}
	rand.Seed(seed)
	nDirected.Initialize(params, numColors, colorsPerConsumer, maxInputConflicts)

	nInput := Network{}
	rand.Seed(seed)
	nInput.Initialize(params, numColors, colorsPerConsumer, maxInputConflicts)

	rand.Seed(seed)
	for i := 0; i < numNodes; i++ {
		if err := nDirected.AddNode(&Directed{}); err != nil {
			t.Fatal(err)
		}
	}

	rand.Seed(seed)
	for i := 0; i < numNodes; i++ {
		if err := nInput.AddNode(&Input{}); err != nil {
			t.Fatal(err)
		}
	}

	for numRounds := 0; !nDirected.Finalized() &&
		!nDirected.Disagreement() &&
		!nInput.Finalized() &&
		!nInput.Disagreement(); numRounds++ {
		rand.Seed(int64(numRounds) + seed)
		if err := nDirected.Round(); err != nil {
			t.Fatal(err)
		}

		rand.Seed(int64(numRounds) + seed)
		if err := nInput.Round(); err != nil {
			t.Fatal(err)
		}
	}

	if nDirected.Disagreement() || nInput.Disagreement() {
		t.Fatalf("Network agreed on inconsistent values")
	}

	if !nDirected.Finalized() ||
		!nInput.Finalized() {
		t.Fatalf("Network agreed on values faster with one of the implementations")
	}
	if !nDirected.Agreement() || !nInput.Agreement() {
		t.Fatalf("Network agreed on inconsistent values")
	}
}
