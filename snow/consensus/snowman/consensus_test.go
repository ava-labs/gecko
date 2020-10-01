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
	GenesisID = ids.Empty.Prefix(0)
	Genesis   = &TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     GenesisID,
		StatusV: choices.Accepted,
	}}

	Tests = []func(*testing.T, Factory){
		InitializeTest,
		AddToTailTest,
		AddToNonTailTest,
		AddToUnknownTest,
		IssuedPreviouslyAcceptedTest,
		IssuedPreviouslyRejectedTest,
		IssuedUnissuedTest,
		IssuedIssuedTest,
		RecordPollAcceptSingleBlockTest,
		RecordPollAcceptAndRejectTest,
		RecordPollWhenFinalizedTest,
		RecordPollRejectTransitivelyTest,
		RecordPollTransitivelyResetConfidenceTest,
		RecordPollInvalidVoteTest,
		RecordPollTransitiveVotingTest,
		RecordPollDivergedVotingTest,
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
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}

	sm.Initialize(ctx, params, GenesisID)

	if p := sm.Parameters(); p != params {
		t.Fatalf("Wrong returned parameters")
	} else if pref := sm.Preference(); !pref.Equals(GenesisID) {
		t.Fatalf("Wrong preference returned")
	} else if !sm.Finalized() {
		t.Fatalf("Wrong should have marked the instance as being finalized")
	}
}

// Make sure that adding a block to the tail updates the preference
func AddToTailTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	// Adding to the previous preference will update the preference
	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if pref := sm.Preference(); !pref.Equals(block.IDV) {
		t.Fatalf("Wrong preference. Expected %s, got %s", block.IDV, pref)
	}
}

// Make sure that adding a block not to the tail doesn't change the preference
func AddToNonTailTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

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
	if rejected, err := sm.Add(firstBlock); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if pref := sm.Preference(); !pref.Equals(firstBlock.IDV) {
		t.Fatalf("Wrong preference. Expected %s, got %s", firstBlock.IDV, pref)
	}

	// Adding to something other than the previous preference won't update the
	// preference
	if rejected, err := sm.Add(secondBlock); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if pref := sm.Preference(); !pref.Equals(firstBlock.IDV) {
		t.Fatalf("Wrong preference. Expected %s, got %s", firstBlock.IDV, pref)
	}
}

// Make sure that adding a block that is detached from the rest of the tree
// rejects the block
func AddToUnknownTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

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
	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if !rejected {
		t.Fatal("should have been rejected")
	} else if pref := sm.Preference(); !pref.Equals(GenesisID) {
		t.Fatalf("Wrong preference. Expected %s, got %s", GenesisID, pref)
	} else if status := block.Status(); status != choices.Rejected {
		t.Fatalf("Should have rejected the block")
	}
}

func IssuedPreviouslyAcceptedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	if !sm.Issued(Genesis) {
		t.Fatalf("Should have marked an accepted block as having been issued")
	}
}

func IssuedPreviouslyRejectedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Rejected,
		},
		ParentV: Genesis,
	}

	if !sm.Issued(block) {
		t.Fatalf("Should have marked a rejected block as having been issued")
	}
}

func IssuedUnissuedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if sm.Issued(block) {
		t.Fatalf("Shouldn't have marked an unissued block as having been issued")
	}
}

func IssuedIssuedTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      3,
		BetaRogue:         5,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if !sm.Issued(block) {
		t.Fatalf("Should have marked a pending block as having been issued")
	} else if rejected {
		t.Fatal("should not have been rejected")
	}
}

func RecordPollAcceptSingleBlockTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      2,
		BetaRogue:         3,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if pref := sm.Preference(); !pref.Equals(block.ID()) {
		t.Fatalf("Preference returned the wrong block")
	} else if sm.Finalized() {
		t.Fatalf("Snowman instance finalized too soon")
	} else if status := block.Status(); status != choices.Processing {
		t.Fatalf("Block's status changed unexpectedly")
	} else if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 0)
	} else if pref := sm.Preference(); !pref.Equals(block.ID()) {
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
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(firstBlock); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(secondBlock); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(firstBlock.ID())

	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if pref := sm.Preference(); !pref.Equals(firstBlock.ID()) {
		t.Fatalf("Preference returned the wrong block")
	} else if sm.Finalized() {
		t.Fatalf("Snowman instance finalized too soon")
	} else if status := firstBlock.Status(); status != choices.Processing {
		t.Fatalf("Block's status changed unexpectedly")
	} else if status := secondBlock.Status(); status != choices.Processing {
		t.Fatalf("Block's status changed unexpectedly")
	} else if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 1 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 1)
	} else if pref := sm.Preference(); !pref.Equals(firstBlock.ID()) {
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
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	votes := ids.Bag{}
	votes.Add(GenesisID)
	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if !sm.Finalized() {
		t.Fatalf("Consensus should still be finalized")
	} else if pref := sm.Preference(); !GenesisID.Equals(pref) {
		t.Fatalf("Wrong preference listed")
	}
}

func RecordPollRejectTransitivelyTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
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
	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 2 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 2)
	}

	// Current graph structure:
	// 0
	// Tail = 0

	if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if pref := sm.Preference(); !block0.ID().Equals(pref) {
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
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      2,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block3); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	// Current graph structure:
	//   G
	//  / \
	// 0   1
	//    / \
	//   2   3

	votesFor2 := ids.Bag{}
	votesFor2.Add(block2.ID())
	if accepted, rejected, err := sm.RecordPoll(votesFor2); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); !block2.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	}

	emptyVotes := ids.Bag{}
	if accepted, rejected, err := sm.RecordPoll(emptyVotes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); !block2.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	} else if accepted, rejected, err := sm.RecordPoll(votesFor2); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); !block2.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	}

	votesFor3 := ids.Bag{}
	votesFor3.Add(block3.ID())
	if accepted, rejected, err := sm.RecordPoll(votesFor3); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 1 { // reject block 0, accept block 1
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 1)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); !block2.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	} else if accepted, rejected, err := sm.RecordPoll(votesFor3); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 1 { // accept block 3, reject block 2
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 1)
	} else if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if pref := sm.Preference(); !block3.ID().Equals(pref) {
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
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      2,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	unknownBlockID := ids.Empty.Prefix(2)

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	validVotes := ids.Bag{}
	validVotes.Add(block.ID())
	if accepted, rejected, err := sm.RecordPoll(validVotes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	}

	invalidVotes := ids.Bag{}
	invalidVotes.Add(unknownBlockID)
	if accepted, rejected, err := sm.RecordPoll(invalidVotes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if accepted, rejected, err := sm.RecordPoll(validVotes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if pref := sm.Preference(); !block.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	}
}

func RecordPollTransitiveVotingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 3,
		Alpha:             3,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block3); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block4); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
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
	if accepted, rejected, err := sm.RecordPoll(votes0_2_4); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 0 { // accept block 0
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 0)
	}

	// Current graph structure:
	//   0
	//  / \
	// 1   3
	// |   |
	// 2   4
	// Tail = 2

	if pref := sm.Preference(); !block2.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	} else if sm.Finalized() {
		t.Fatalf("Finalized too early")
	} else if block0.Status() != choices.Accepted {
		t.Fatalf("Should have accepted")
	} else if block1.Status() != choices.Processing {
		t.Fatalf("Should have accepted")
	} else if block2.Status() != choices.Processing {
		t.Fatalf("Should have accepted")
	} else if block3.Status() != choices.Processing {
		t.Fatalf("Should have rejected")
	} else if block4.Status() != choices.Processing {
		t.Fatalf("Should have rejected")
	}

	dep2_2_2 := ids.Bag{}
	dep2_2_2.AddCount(block2.ID(), 3)
	if accepted, rejected, err := sm.RecordPoll(dep2_2_2); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 2 || rejected.Len() != 2 { // accept blocks 1 and 2, reject blocks 3 and 4
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 2, 2)
	}

	// Current graph structure:
	//   2
	// Tail = 2

	if pref := sm.Preference(); !block2.ID().Equals(pref) {
		t.Fatalf("Wrong preference listed")
	} else if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if block0.Status() != choices.Accepted {
		t.Fatalf("Should have accepted")
	} else if block1.Status() != choices.Accepted {
		t.Fatalf("Should have accepted")
	} else if block2.Status() != choices.Accepted {
		t.Fatalf("Should have accepted")
	} else if block3.Status() != choices.Rejected {
		t.Fatalf("Should have rejected")
	} else if block4.Status() != choices.Rejected {
		t.Fatalf("Should have rejected")
	}
}

func RecordPollDivergedVotingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	sm.Initialize(ctx, params, GenesisID)

	block0 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.NewID([32]byte{0x0f}), // 0b1111
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block1 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.NewID([32]byte{0x08}), // 0b1000
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}
	block2 := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.NewID([32]byte{0x01}), // 0b0001
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

	if rejected, err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes0 := ids.Bag{}
	votes0.Add(block0.ID())
	if accepted, rejected, err := sm.RecordPoll(votes0); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if rejected, err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	// block2 can't be accepted.

	if rejected, err := sm.Add(block3); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if status := block0.Status(); status == choices.Accepted {
		t.Fatalf("Shouldn't be accepted yet")
	}

	// Transitively increases dep2. However, dep2 shares the first bit with
	// dep0.
	votes3 := ids.Bag{}
	votes3.Add(block3.ID())
	if accepted, rejected, err := sm.RecordPoll(votes3); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 3 { // accept block 0, reject blocks 1,2,3
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 3)
	} else if !sm.Finalized() {
		t.Fatalf("Finalized too late")
	} else if status := block0.Status(); status != choices.Accepted {
		t.Fatalf("Should be accepted")
	}
}

func MetricsProcessingErrorTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	numProcessing := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "processing",
		})

	if err := params.Metrics.Register(numProcessing); err != nil {
		t.Fatal(err)
	}

	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if !sm.Finalized() {
		t.Fatalf("Snowman instance didn't finalize")
	}
}

func MetricsAcceptedErrorTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	numAccepted := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "accepted",
		})

	if err := params.Metrics.Register(numAccepted); err != nil {
		t.Fatal(err)
	}

	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	} else if !sm.Finalized() {
		t.Fatalf("Snowman instance didn't finalize")
	}
}

func MetricsRejectedErrorTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	numRejected := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "rejected",
		})

	if err := params.Metrics.Register(numRejected); err != nil {
		t.Fatal(err)
	}

	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if accepted.Len() != 1 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 1, 0)
	} else if !sm.Finalized() {
		t.Fatalf("Snowman instance didn't finalize")
	}
}

func ErrorOnInitialRejectionTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(block); err == nil {
		t.Fatalf("Should have errored on rejecting the rejectable block")
	} else if !rejected {
		t.Fatal("should have been rejected")
	}
}

func ErrorOnAcceptTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	sm.Initialize(ctx, params, GenesisID)

	block := &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(1),
			AcceptV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
	}

	if rejected, err := sm.Add(block); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on accepted the block")
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	}
}

func ErrorOnRejectSiblingTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block0.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on rejecting the block's sibling")
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	}
}

func ErrorOnTransitiveRejectionTest(t *testing.T, factory Factory) {
	sm := factory.New()

	ctx := snow.DefaultContextTest()
	params := snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}

	sm.Initialize(ctx, params, GenesisID)

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

	if rejected, err := sm.Add(block0); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block1); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	} else if rejected, err := sm.Add(block2); err != nil {
		t.Fatal(err)
	} else if rejected {
		t.Fatal("should not have been rejected")
	}

	votes := ids.Bag{}
	votes.Add(block0.ID())
	if accepted, rejected, err := sm.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on transitively rejecting the block")
	} else if accepted.Len() != 0 || rejected.Len() != 0 {
		t.Fatalf("accepted/rejected %d/%d blocks but should be %d/%d", accepted.Len(), rejected.Len(), 0, 0)
	}
}

func RandomizedConsistencyTest(t *testing.T, factory Factory) {
	numColors := 50
	numNodes := 100
	params := snowball.Parameters{
		Metrics:      prometheus.NewRegistry(),
		K:            20,
		Alpha:        15,
		BetaVirtuous: 20,
		BetaRogue:    30,
	}
	seed := int64(0)

	rand.Seed(seed)

	n := Network{}
	n.Initialize(params, numColors)

	for i := 0; i < numNodes; i++ {
		n.AddNode(factory.New())
	}

	for !n.Finalized() {
		n.Round()
	}

	if !n.Agreement() {
		t.Fatalf("Network agreed on inconsistent values")
	}
}
