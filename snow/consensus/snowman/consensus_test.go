// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

var (
	GenesisID     = ids.Empty.Prefix(0)
	GenesisHeight = uint64(0)
	Genesis       = &TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     GenesisID,
		StatusV: choices.Accepted,
	}}

	Tests = []func(*testing.T, Factory){
		InitializeTest,
		NumProcessingTest,
		AddToTailTest,
		AddToNonTailTest,
		AddToUnknownTest,
		StatusOrProcessingPreviouslyAcceptedTest,
		StatusOrProcessingPreviouslyRejectedTest,
		StatusOrProcessingUnissuedTest,
		StatusOrProcessingIssuedTest,
		RecordPollAcceptSingleBlockTest,
		RecordPollAcceptAndRejectTest,
		RecordPollWhenFinalizedTest,
		RecordPollRejectTransitivelyTest,
		RecordPollTransitivelyResetConfidenceTest,
		RecordPollInvalidVoteTest,
		RecordPollTransitiveVotingTest,
		RecordPollDivergedVotingTest,
		RecordPollChangePreferredChainTest,
		MetricsProcessingErrorTest,
		MetricsAcceptedErrorTest,
		MetricsRejectedErrorTest,
		ErrorOnInitialRejectionTest,
		ErrorOnAcceptTest,
		ErrorOnRejectSiblingTest,
		ErrorOnTransitiveRejectionTest,
		RandomizedConsistencyTest,
	}
)

// Execute all tests against a consensus implementation
func ConsensusTest(t *testing.T, factory Factory) {
	for _, test := range Tests {
		test(t, factory)
	}
}

// Make sure that initialize sets the state correctly
func InitializeTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	if p := sm.Parameters(); p != params {
		t.Fatalf("Wrong returned parameters")
	} else if pref := sm.Preference(); pref != GenesisID {
		t.Fatalf("Wrong preference returned")
	} else if !sm.Finalized() {
		t.Fatalf("Wrong should have marked the instance as being finalized")
	}
}

// Make sure that the number of processing blocks is tracked correctly
func NumProcessingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if numProcessing := sm.NumProcessing(); numProcessing != 0 {
		t.Fatalf("expected %d blocks to be processing but returned %d", 0, numProcessing)
	}

	// Adding to the previous preference will update the preference
	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	}

	if numProcessing := sm.NumProcessing(); numProcessing != 1 {
		t.Fatalf("expected %d blocks to be processing but returned %d", 1, numProcessing)
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	}

	if numProcessing := sm.NumProcessing(); numProcessing != 0 {
		t.Fatalf("expected %d blocks to be processing but returned %d", 0, numProcessing)
	}
}

// Make sure that adding a block to the tail updates the preference
func AddToTailTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	// Adding to the previous preference will update the preference
	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != block.ID() {
		t.Fatalf("Wrong preference. Expected %s, got %s", block.ID(), pref)
	} else if !sm.IsPreferred(block) {
		t.Fatalf("Should have marked %s as being Preferred", pref)
	}
}

// Make sure that adding a block not to the tail doesn't change the preference
func AddToNonTailTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	firstBlock := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	secondBlock := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	// Adding to the previous preference will update the preference
	if err := sm.Add(firstBlock); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != firstBlock.IDV {
		t.Fatalf("Wrong preference. Expected %s, got %s", firstBlock.IDV, pref)
	}

	// Adding to something other than the previous preference won't update the
	// preference
	if err := sm.Add(secondBlock); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != firstBlock.IDV {
		t.Fatalf("Wrong preference. Expected %s, got %s", firstBlock.IDV, pref)
	}
}

// Make sure that adding a block that is detached from the rest of the tree
// rejects the block
func AddToUnknownTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	parent := &TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.Empty.Prefix(1),
		StatusV: choices.Unknown,
	}}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: parent,
	}

	// Adding a block with an unknown parent means the parent must have already
	// been rejected. Therefore the block should be immediately rejected
	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != GenesisID {
		t.Fatalf("Wrong preference. Expected %s, got %s", GenesisID, pref)
	} else if status := block.Status(); status != choices.Rejected {
		t.Fatalf("Should have rejected the block")
	}
}

func StatusOrProcessingPreviouslyAcceptedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	if !sm.AcceptedOrProcessing(Genesis) {
		t.Fatalf("Should have marked an accepted block as having been decided")
	}
	if !sm.DecidedOrProcessing(Genesis) {
		t.Fatalf("Should have marked an accepted block as having been decided")
	}
	if !sm.IsPreferred(Genesis) {
		t.Fatalf("Should have marked an accepted block as being preferred")
	}
}

func StatusOrProcessingPreviouslyRejectedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Rejected,
		},
		ParentV: Genesis,
	}

	if sm.AcceptedOrProcessing(block) {
		t.Fatalf("Shouldn't have marked a rejected block as having been accepted")
	}
	if !sm.DecidedOrProcessing(block) {
		t.Fatalf("Should have marked a rejected block as having been decided")
	}
	if sm.IsPreferred(block) {
		t.Fatalf("Shouldn't have marked a rejected block as being preferred")
	}
}

func StatusOrProcessingUnissuedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
		HeightV: 1,
	}

	if sm.AcceptedOrProcessing(block) {
		t.Fatalf("Shouldn't have marked an unissued block as being processing")
	}
	if sm.DecidedOrProcessing(block) {
		t.Fatalf("Shouldn't have marked an unissued block as being processing")
	}
	if sm.IsPreferred(block) {
		t.Fatalf("Shouldn't have marked an unissued block as being preferred")
	}
}

func StatusOrProcessingIssuedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          3,
		BetaRogue:             5,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	}
	if !sm.AcceptedOrProcessing(block) {
		t.Fatalf("Should have marked a the block as processing")
	}
	if !sm.DecidedOrProcessing(block) {
		t.Fatalf("Should have marked a the block as processing")
	}
	if !sm.IsPreferred(block) {
		t.Fatalf("Should have marked the tail as being preferred")
	}
}

func RecordPollAcceptSingleBlockTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          2,
		BetaRogue:             3,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != block.ID() {
		t.Fatalf("Preference returned the wrong block")
	} else if sm.Finalized() {
		t.Fatalf("Snowman instance finalized too soon")
	} else if status := block.Status(); status != choices.Processing {
		t.Fatalf("Block's status changed unexpectedly")
	} else if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != block.ID() {
		t.Fatalf("Preference returned the wrong block")
	} else if !sm.Finalized() {
		t.Fatalf("Snowman instance didn't finalize")
	} else if status := block.Status(); status != choices.Accepted {
		t.Fatalf("Block's status should have been set to accepted")
	}
}

func RecordPollAcceptAndRejectTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             2,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	firstBlock := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	secondBlock := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if err := sm.Add(firstBlock); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(secondBlock); err != nil {
		t.Fatal(err)
	}

	votes := ids.Bag{}
	votes.Add(firstBlock.ID())

	if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != firstBlock.ID() {
		t.Fatalf("Preference returned the wrong block")
	} else if sm.Finalized() {
		t.Fatalf("Snowman instance finalized too soon")
	} else if status := firstBlock.Status(); status != choices.Processing {
		t.Fatalf("Block's status changed unexpectedly")
	} else if status := secondBlock.Status(); status != choices.Processing {
		t.Fatalf("Block's status changed unexpectedly")
	} else if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if pref := sm.Preference(); pref != firstBlock.ID() {
		t.Fatalf("Preference returned the wrong block")
	} else if !sm.Finalized() {
		t.Fatalf("Snowman instance didn't finalize")
	} else if status := firstBlock.Status(); status != choices.Accepted {
		t.Fatalf("Block's status should have been set to accepted")
	} else if status := secondBlock.Status(); status != choices.Rejected {
		t.Fatalf("Block's status should have been set to rejected")
	}
}

func RecordPollWhenFinalizedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             2,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	votes := ids.Bag{}
	votes.Add(GenesisID)
	if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if !sm.Finalized() {
		t.Fatalf("Consensus should still be finalized")
	} else if pref := sm.Preference(); GenesisID != pref {
		t.Fatalf("Wrong preference listed")
	}
}

func RecordPollRejectTransitivelyTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(3),
			StatusV: choices.Processing,
		},
		ParentV: block1,
	}

	if err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block2); err != nil {
		t.Fatal(err)
	}

	// Current graph structure:
	//   G
	//  / \
	// 0   1
	//     |
	//     2
	// Tail = 0

	votes := ids.Bag{}
	votes.Add(block0.ID())
	if err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	}

	// Current graph structure:
	// 0
	// Tail = 0

	if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if pref := sm.Preference(); block0.ID() != pref {
		t.Fatalf("Wrong preference listed")
	} else if status := block0.Status(); status != choices.Accepted {
		t.Fatalf("Wrong status returned")
	} else if status := block1.Status(); status != choices.Rejected {
		t.Fatalf("Wrong status returned")
	} else if status := block2.Status(); status != choices.Rejected {
		t.Fatalf("Wrong status returned")
	}
}

func RecordPollTransitivelyResetConfidenceTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          2,
		BetaRogue:             2,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(3),
			StatusV: choices.Processing,
		},
		ParentV: block1,
	}
	block3 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(4),
			StatusV: choices.Processing,
		},
		ParentV: block1,
	}

	if err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block3); err != nil {
		t.Fatal(err)
	}

	// Current graph structure:
	//   G
	//  / \
	// 0   1
	//    / \
	//   2   3

	votesFor2 := ids.Bag{}
	votesFor2.Add(block2.ID())
	if err := sm.RecordPoll(votesFor2); err != nil {
		t.Fatal(err)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); block2.ID() != pref {
		t.Fatalf("Wrong preference listed")
	}

	emptyVotes := ids.Bag{}
	if err := sm.RecordPoll(emptyVotes); err != nil {
		t.Fatal(err)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); block2.ID() != pref {
		t.Fatalf("Wrong preference listed")
	} else if err := sm.RecordPoll(votesFor2); err != nil {
		t.Fatal(err)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); block2.ID() != pref {
		t.Fatalf("Wrong preference listed")
	}

	votesFor3 := ids.Bag{}
	votesFor3.Add(block3.ID())
	if err := sm.RecordPoll(votesFor3); err != nil {
		t.Fatal(err)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); block2.ID() != pref {
		t.Fatalf("Wrong preference listed")
	} else if err := sm.RecordPoll(votesFor3); err != nil {
		t.Fatal(err)
	} else if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if pref := sm.Preference(); block3.ID() != pref {
		t.Fatalf("Wrong preference listed")
	} else if status := block0.Status(); status != choices.Rejected {
		t.Fatalf("Wrong status returned")
	} else if status := block1.Status(); status != choices.Accepted {
		t.Fatalf("Wrong status returned")
	} else if status := block2.Status(); status != choices.Rejected {
		t.Fatalf("Wrong status returned")
	} else if status := block3.Status(); status != choices.Accepted {
		t.Fatalf("Wrong status returned")
	}
}

func RecordPollInvalidVoteTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          2,
		BetaRogue:             2,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	unknownBlockID := ids.Empty.Prefix(2)

	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	}

	validVotes := ids.Bag{}
	validVotes.Add(block.ID())
	if err := sm.RecordPoll(validVotes); err != nil {
		t.Fatal(err)
	}

	invalidVotes := ids.Bag{}
	invalidVotes.Add(unknownBlockID)
	if err := sm.RecordPoll(invalidVotes); err != nil {
		t.Fatal(err)
	} else if err := sm.RecordPoll(validVotes); err != nil {
		t.Fatal(err)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); block.ID() != pref {
		t.Fatalf("Wrong preference listed")
	}
}

func RecordPollTransitiveVotingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     3,
		Alpha:                 3,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: block0,
	}
	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(3),
			StatusV: choices.Processing,
		},
		ParentV: block1,
	}
	block3 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(4),
			StatusV: choices.Processing,
		},
		ParentV: block0,
	}
	block4 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(5),
			StatusV: choices.Processing,
		},
		ParentV: block3,
	}

	if err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block3); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block4); err != nil {
		t.Fatal(err)
	}

	// Current graph structure:
	//   G
	//   |
	//   0
	//  / \
	// 1   3
	// |   |
	// 2   4
	// Tail = 2

	votes0_2_4 := ids.Bag{}
	votes0_2_4.Add(
		block0.ID(),
		block2.ID(),
		block4.ID(),
	)
	if err := sm.RecordPoll(votes0_2_4); err != nil {
		t.Fatal(err)
	}

	// Current graph structure:
	//   0
	//  / \
	// 1   3
	// |   |
	// 2   4
	// Tail = 2

	pref := sm.Preference()
	switch {
	case block2.ID() != pref:
		t.Fatalf("Wrong preference listed")
	case sm.Finalized():
		t.Fatalf("Finalized too early")
	case block0.Status() != choices.Accepted:
		t.Fatalf("Should have accepted")
	case block1.Status() != choices.Processing:
		t.Fatalf("Should have accepted")
	case block2.Status() != choices.Processing:
		t.Fatalf("Should have accepted")
	case block3.Status() != choices.Processing:
		t.Fatalf("Should have rejected")
	case block4.Status() != choices.Processing:
		t.Fatalf("Should have rejected")
	}

	dep2_2_2 := ids.Bag{}
	dep2_2_2.AddCount(block2.ID(), 3)
	if err := sm.RecordPoll(dep2_2_2); err != nil {
		t.Fatal(err)
	}

	// Current graph structure:
	//   2
	// Tail = 2

	pref = sm.Preference()
	switch {
	case block2.ID() != pref:
		t.Fatalf("Wrong preference listed")
	case !sm.Finalized():
		t.Fatalf("Finalized too late")
	case block0.Status() != choices.Accepted:
		t.Fatalf("Should have accepted")
	case block1.Status() != choices.Accepted:
		t.Fatalf("Should have accepted")
	case block2.Status() != choices.Accepted:
		t.Fatalf("Should have accepted")
	case block3.Status() != choices.Rejected:
		t.Fatalf("Should have rejected")
	case block4.Status() != choices.Rejected:
		t.Fatalf("Should have rejected")
	}
}

func RecordPollDivergedVotingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             2,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.ID{0x0f}, // 0b1111
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.ID{0x08}, // 0b1000
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.ID{0x01}, // 0b0001
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block3 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: block2,
	}

	if err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block1); err != nil {
		t.Fatal(err)
	}

	votes0 := ids.Bag{}
	votes0.Add(block0.ID())
	if err := sm.RecordPoll(votes0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block2); err != nil {
		t.Fatal(err)
	}

	// dep2 is already rejected.

	if err := sm.Add(block3); err != nil {
		t.Fatal(err)
	} else if status := block0.Status(); status == choices.Accepted {
		t.Fatalf("Shouldn't be accepted yet")
	}

	// Transitively increases dep2. However, dep2 shares the first bit with
	// dep0. Because dep2 is already rejected, this will accept dep0.
	votes3 := ids.Bag{}
	votes3.Add(block3.ID())
	if err := sm.RecordPoll(votes3); err != nil {
		t.Fatal(err)
	} else if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if status := block0.Status(); status != choices.Accepted {
		t.Fatalf("Should be accepted")
	}
}

func RecordPollChangePreferredChainTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          10,
		BetaRogue:             10,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	a1Block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
		HeightV: 1,
	}
	b1Block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
		HeightV: 1,
	}
	a2Block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: a1Block,
		HeightV: 2,
	}
	b2Block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: b1Block,
		HeightV: 2,
	}

	if err := sm.Add(a1Block); err != nil {
		t.Fatal(err)
	}
	if err := sm.Add(a2Block); err != nil {
		t.Fatal(err)
	}
	if err := sm.Add(b1Block); err != nil {
		t.Fatal(err)
	}
	if err := sm.Add(b2Block); err != nil {
		t.Fatal(err)
	}

	if sm.Preference() != a2Block.ID() {
		t.Fatal("Wrong preference reported")
	}

	if !sm.IsPreferred(a1Block) {
		t.Fatalf("Should have reported a1 as being preferred")
	}
	if !sm.IsPreferred(a2Block) {
		t.Fatalf("Should have reported a2 as being preferred")
	}
	if sm.IsPreferred(b1Block) {
		t.Fatalf("Shouldn't have reported b1 as being preferred")
	}
	if sm.IsPreferred(b2Block) {
		t.Fatalf("Shouldn't have reported b2 as being preferred")
	}

	b2Votes := ids.Bag{}
	b2Votes.Add(b2Block.ID())

	if err := sm.RecordPoll(b2Votes); err != nil {
		t.Fatal(err)
	}

	if sm.Preference() != b2Block.ID() {
		t.Fatal("Wrong preference reported")
	}

	if sm.IsPreferred(a1Block) {
		t.Fatalf("Shouldn't have reported a1 as being preferred")
	}
	if sm.IsPreferred(a2Block) {
		t.Fatalf("Shouldn't have reported a2 as being preferred")
	}
	if !sm.IsPreferred(b1Block) {
		t.Fatalf("Should have reported b1 as being preferred")
	}
	if !sm.IsPreferred(b2Block) {
		t.Fatalf("Should have reported b2 as being preferred")
	}

	a1Votes := ids.Bag{}
	a1Votes.Add(a1Block.ID())

	if err := sm.RecordPoll(a1Votes); err != nil {
		t.Fatal(err)
	}
	if err := sm.RecordPoll(a1Votes); err != nil {
		t.Fatal(err)
	}

	if sm.Preference() != a2Block.ID() {
		t.Fatal("Wrong preference reported")
	}

	if !sm.IsPreferred(a1Block) {
		t.Fatalf("Should have reported a1 as being preferred")
	}
	if !sm.IsPreferred(a2Block) {
		t.Fatalf("Should have reported a2 as being preferred")
	}
	if sm.IsPreferred(b1Block) {
		t.Fatalf("Shouldn't have reported b1 as being preferred")
	}
	if sm.IsPreferred(b2Block) {
		t.Fatalf("Shouldn't have reported b2 as being preferred")
	}
}

func MetricsProcessingErrorTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	numProcessing := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "blks_processing",
		})

	if err := params.Metrics.Register(numProcessing); err != nil {
		t.Fatal(err)
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err == nil {
		t.Fatalf("should have errored during initialization due to a duplicate metric")
	}
}

func MetricsAcceptedErrorTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	numAccepted := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "blks_accepted",
		})

	if err := params.Metrics.Register(numAccepted); err != nil {
		t.Fatal(err)
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err == nil {
		t.Fatalf("should have errored during initialization due to a duplicate metric")
	}
}

func MetricsRejectedErrorTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	numRejected := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "blks_rejected",
		})

	if err := params.Metrics.Register(numRejected); err != nil {
		t.Fatal(err)
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err == nil {
		t.Fatalf("should have errored during initialization due to a duplicate metric")
	}
}

func ErrorOnInitialRejectionTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	rejectedBlock := &TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.Empty.Prefix(1),
		StatusV: choices.Rejected,
	}}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentV: rejectedBlock,
	}

	if err := sm.Add(block); err == nil {
		t.Fatalf("Should have errored on rejecting the rejectable block")
	}
}

func ErrorOnAcceptTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			AcceptV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if err := sm.Add(block); err != nil {
		t.Fatal(err)
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if err := sm.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on accepted the block")
	}
}

func ErrorOnRejectSiblingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block1); err != nil {
		t.Fatal(err)
	}

	votes := ids.Bag{}
	votes.Add(block0.ID())
	if err := sm.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on rejecting the block's sibling")
	}
}

func ErrorOnTransitiveRejectionTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     1,
		Alpha:                 1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}

	if err := sm.Initialize(ctx, params, GenesisID, GenesisHeight); err != nil {
		t.Fatal(err)
	}

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(2),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(3),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentV: block1,
	}

	if err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if err := sm.Add(block2); err != nil {
		t.Fatal(err)
	}

	votes := ids.Bag{}
	votes.Add(block0.ID())
	if err := sm.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on transitively rejecting the block")
	}
}

func RandomizedConsistencyTest(t *testing.T, factory Factory) {
	numColors := 50
	numNodes := 100
	params := snowball.Parameters{
		Metrics:               prometheus.NewRegistry(),
		K:                     20,
		Alpha:                 15,
		BetaVirtuous:          20,
		BetaRogue:             30,
		ConcurrentRepolls:     1,
		OptimalProcessing:     1,
		MaxOutstandingItems:   1,
		MaxItemProcessingTime: 1,
	}
	seed := int64(0)

	rand.Seed(seed)

	n := Network{}
	n.Initialize(params, numColors)

	for i := 0; i < numNodes; i++ {
		if err := n.AddNode(factory.New()); err != nil {
			t.Fatal(err)
		}
	}

	for !n.Finalized() {
		if err := n.Round(); err != nil {
			t.Fatal(err)
		}
	}

	if !n.Agreement() {
		t.Fatalf("Network agreed on inconsistent values")
	}
}
