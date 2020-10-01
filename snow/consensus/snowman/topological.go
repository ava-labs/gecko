// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

const (
	minMapSize = 16
)

// TopologicalFactory implements Factory by returning a topological struct
type TopologicalFactory struct{}

// New implements Factory
func (TopologicalFactory) New() Consensus { return &Topological{} }

// Topological implements the Snowman interface by using a tree tracking the
// strongly preferred branch. This tree structure amortizes network polls to
// vote on more than just the next block.
type Topological struct {
	metrics

	// ctx is the context this snowman instance is executing in
	ctx *snow.Context

	// params are the parameters that should be used to initialize snowball
	// instances
	params snowball.Parameters

	// head is the last accepted block
	head ids.ID

	// blocks stores the last accepted block and all the pending blocks
	blocks map[[32]byte]*snowmanBlock // blockID -> snowmanBlock

	// tail is the preferred block with no children
	tail ids.ID
}

// Used to track the kahn topological sort status
type kahnNode struct {
	// inDegree is the number of children that haven't been processed yet. If
	// inDegree is 0, then this node is a leaf
	inDegree int
	// votes for all the children of this node, so far
	votes ids.Bag
}

// Used to track which children should receive votes
type votes struct {
	// parentID is the parent of all the votes provided in the votes bag
	parentID ids.ID
	// votes for all the children of the parent
	votes ids.Bag
}

// Initialize implements the Snowman interface
func (ts *Topological) Initialize(ctx *snow.Context, params snowball.Parameters, rootID ids.ID) {
	ts.ctx = ctx
	ts.params = params

	if err := ts.metrics.Initialize(ctx.Log, params.Namespace, params.Metrics); err != nil {
		ts.ctx.Log.Error("%s", err)
	}

	ts.head = rootID
	ts.blocks = map[[32]byte]*snowmanBlock{
		rootID.Key(): {sm: ts},
	}
	ts.tail = rootID
}

// Parameters implements the Snowman interface
func (ts *Topological) Parameters() snowball.Parameters { return ts.params }

// Add implements the Snowman interface
func (ts *Topological) Add(blk Block) (bool, error) {
	parentID := blk.Parent()
	parentKey := parentID.Key()

	blkID := blk.ID()
	blkBytes := blk.Bytes()

	// Notify anyone listening that this block was issued.
	ts.ctx.DecisionDispatcher.Issue(ts.ctx, blkID, blkBytes)
	ts.ctx.ConsensusDispatcher.Issue(ts.ctx, blkID, blkBytes)
	ts.metrics.Issued(blkID)

	parentNode, ok := ts.blocks[parentKey]
	if !ok {
		// If the ancestor is missing, this means the ancestor must have already
		// been pruned. Therefore, the dependent should be transitively
		// rejected.
		if err := blk.Reject(); err != nil {
			return true, err
		}

		// Notify anyone listening that this block was rejected.
		ts.ctx.DecisionDispatcher.Reject(ts.ctx, blkID, blkBytes)
		ts.ctx.ConsensusDispatcher.Reject(ts.ctx, blkID, blkBytes)
		ts.metrics.Rejected(blkID)
		return true, nil
	}

	// add the block as a child of its parent, and add the block to the tree
	parentNode.AddChild(blk)
	ts.blocks[blkID.Key()] = &snowmanBlock{
		sm:  ts,
		blk: blk,
	}

	// If we are extending the tail, this is the new tail
	if ts.tail.Equals(parentID) {
		ts.tail = blkID
	}
	return false, nil
}

// Issued implements the Snowman interface
func (ts *Topological) Issued(blk Block) bool {
	// If the block is decided, then it must have been previously issued.
	if blk.Status().Decided() {
		return true
	}
	// If the block is in the map of current blocks, then the block was issued.
	_, ok := ts.blocks[blk.ID().Key()]
	return ok
}

// Preference implements the Snowman interface
func (ts *Topological) Preference() ids.ID { return ts.tail }

// RecordPoll implements the Snowman interface
//
// The votes bag contains at most K votes for blocks in the tree. If there is a
// vote for a block that isn't in the tree, the vote is dropped.
//
// Votes are propagated transitively towards the genesis. All blocks in the tree
// that result in at least Alpha votes will record the poll on their children.
// Every other block will have an unsuccessful poll registered.
//
// After collecting which blocks should be voted on, the polls are registered
// and blocks are accepted/rejected as needed. The tail is then updated to equal
// the leaf on the preferred branch.
//
// To optimize the theoretical complexity of the vote propagation, a topological
// sort is done over the blocks that are reachable from the provided votes.
// During the sort, votes are pushed towards the genesis. To prevent interating
// over all blocks that had unsuccessful polls, we set a flag on the block to
// know that any future traversal through that block should register an
// unsuccessful poll on that block and every descendant block.
//
// The complexity of this function is:
// - Runtime = 3 * |live set| + |votes|
// - Space = 2 * |live set| + |votes|
//
// Returns:
//   1) IDs of accepted vertices, or the empty set if there are none
//   2) IDs of rejected vertices, or the empty set if there are none
// If an error is returned, both of the above returned sets are nil
func (ts *Topological) RecordPoll(voteBag ids.Bag) (ids.Set, ids.Set, error) {
	var voteStack []votes
	if voteBag.Len() >= ts.params.Alpha {
		// If there is no way for an alpha majority to occur, there is no need
		// to perform any traversals.

		// Runtime = |live set| + |votes| ; Space = |live set| + |votes|
		kahnGraph, leaves := ts.calculateInDegree(voteBag)

		// Runtime = |live set| ; Space = |live set|
		voteStack = ts.pushVotes(kahnGraph, leaves)
	}

	// Runtime = |live set| ; Space = Constant
	preferred, accepted, rejected, err := ts.vote(voteStack)
	if err != nil {
		return nil, nil, err
	}

	// Runtime = |live set| ; Space = Constant
	ts.tail = ts.getPreferredDecendent(preferred)
	return accepted, rejected, nil
}

// Finalized implements the Snowman interface
func (ts *Topological) Finalized() bool { return len(ts.blocks) == 1 }

// takes in a list of votes and sets up the topological ordering. Returns the
// reachable section of the graph annotated with the number of inbound edges and
// the non-transitively applied votes. Also returns the list of leaf blocks.
func (ts *Topological) calculateInDegree(
	votes ids.Bag) (map[[32]byte]kahnNode, []ids.ID) {
	kahns := make(map[[32]byte]kahnNode, minMapSize)
	leaves := ids.Set{}

	for _, vote := range votes.List() {
		voteKey := vote.Key()
		votedBlock, validVote := ts.blocks[voteKey]

		// If the vote is for a block that isn't in the current pending set,
		// then the vote is dropped
		if !validVote {
			continue
		}

		// If the vote is for the last accepted block, the vote is dropped
		if votedBlock.Accepted() {
			continue
		}

		// The parent contains the snowball instance of its children
		parentID := votedBlock.blk.Parent()
		parentIDKey := parentID.Key()

		// Add the votes for this block to the parent's set of responces
		numVotes := votes.Count(vote)
		kahn, previouslySeen := kahns[parentIDKey]
		kahn.votes.AddCount(vote, numVotes)
		kahns[parentIDKey] = kahn

		// If the parent block already had registered votes, then there is no
		// need to iterate into the parents
		if previouslySeen {
			continue
		}

		// If I've never seen this parent block before, it is currently a leaf.
		leaves.Add(parentID)

		// iterate through all the block's ancestors and set up the inDegrees of
		// the blocks
		for n := ts.blocks[parentIDKey]; !n.Accepted(); n = ts.blocks[parentIDKey] {
			parentID := n.blk.Parent()
			parentIDKey = parentID.Key() // move the loop variable forward

			// Increase the inDegree by one
			kahn := kahns[parentIDKey]
			kahn.inDegree++
			kahns[parentIDKey] = kahn

			// If we have already seen this block, then we shouldn't increase
			// the inDegree of the ancestors through this block again.
			if kahn.inDegree != 1 {
				break
			}

			// If I am transitively seeing this block for the first time, either
			// the block was previously unknown or it was previously a leaf.
			// Regardless, it shouldn't be tracked as a leaf.
			leaves.Remove(parentID)
		}
	}

	return kahns, leaves.List()
}

// convert the tree into a branch of snowball instances with at least alpha
// votes
func (ts *Topological) pushVotes(
	kahnNodes map[[32]byte]kahnNode, leaves []ids.ID) []votes {
	voteStack := make([]votes, 0, len(kahnNodes))
	for len(leaves) > 0 {
		// pop a leaf off the stack
		newLeavesSize := len(leaves) - 1
		leafID := leaves[newLeavesSize]
		leaves = leaves[:newLeavesSize]

		// get the block and sort infomation about the block
		leafIDKey := leafID.Key()
		kahnNode := kahnNodes[leafIDKey]
		block := ts.blocks[leafIDKey]

		// If there are at least Alpha votes, then this block needs to record
		// the poll on the snowball instance
		if kahnNode.votes.Len() >= ts.params.Alpha {
			voteStack = append(voteStack, votes{
				parentID: leafID,
				votes:    kahnNode.votes,
			})
		}

		// If the block is accepted, then we don't need to push votes to the
		// parent block
		if block.Accepted() {
			continue
		}

		parentID := block.blk.Parent()
		parentIDKey := parentID.Key()

		// Remove an inbound edge from the parent kahn node and push the votes.
		parentKahnNode := kahnNodes[parentIDKey]
		parentKahnNode.inDegree--
		parentKahnNode.votes.AddCount(leafID, kahnNode.votes.Len())
		kahnNodes[parentIDKey] = parentKahnNode

		// If the inDegree is zero, then the parent node is now a leaf
		if parentKahnNode.inDegree == 0 {
			leaves = append(leaves, parentID)
		}
	}
	return voteStack
}

// apply votes to the branch that received an Alpha threshold
// Returns:
//   1) The tail
//   2) IDs of accepted blocks, or the empty set if there are none
//   3) IDs of rejected blocks, or the empty set if there are none
// If an error is returned, (2) and (3) are both nil
func (ts *Topological) vote(voteStack []votes) (ids.ID, ids.Set, ids.Set, error) {
	// If the voteStack is empty, then the full tree should falter. This won't
	// change the preferred branch.
	if len(voteStack) == 0 {
		ts.ctx.Log.Verbo("No progress was made after a vote with %d pending blocks", len(ts.blocks)-1)

		headKey := ts.head.Key()
		headBlock := ts.blocks[headKey]
		headBlock.shouldFalter = true
		return ts.tail, nil, nil, nil
	}

	// Keep track of accepted/rejected blocks
	var accepted ids.Set
	var rejected ids.Set
	// keep track of the new preferred block
	newPreferred := ts.head
	onPreferredBranch := true
	for len(voteStack) > 0 {
		// pop a vote off the stack
		newStackSize := len(voteStack) - 1
		vote := voteStack[newStackSize]
		voteStack = voteStack[:newStackSize]

		// get the block that we are going to vote on
		voteParentIDKey := vote.parentID.Key()
		parentBlock, notRejected := ts.blocks[voteParentIDKey]

		// if the block block we are going to vote on was already rejected, then
		// we should stop applying the votes
		if !notRejected {
			break
		}

		// keep track of transitive falters to propagate to this block's
		// children
		shouldTransitivelyFalter := parentBlock.shouldFalter

		// if the block was previously marked as needing to falter, the block
		// should falter before applying the vote
		if shouldTransitivelyFalter {
			ts.ctx.Log.Verbo("Resetting confidence below %s", vote.parentID)

			parentBlock.sb.RecordUnsuccessfulPoll()
			parentBlock.shouldFalter = false
		}

		// apply the votes for this snowball instance
		parentBlock.sb.RecordPoll(vote.votes)

		// Only accept when you are finalized and the head.
		if parentBlock.sb.Finalized() && ts.head.Equals(vote.parentID) {
			acc, rej, err := ts.accept(parentBlock)
			if err != nil {
				return ids.ID{}, nil, nil, err
			}
			accepted.Add(acc)
			rejected.Union(rej)

			// by accepting the child of parentBlock, the last accepted block is
			// no longer voteParentID, but its child. So, voteParentID can be
			// removed from the tree.
			delete(ts.blocks, voteParentIDKey)
		}

		// If we are on the preferred branch, then the parent's preference is
		// the next block on the preferred branch.
		parentPreference := parentBlock.sb.Preference()
		if onPreferredBranch {
			newPreferred = parentPreference
		}

		// Get the ID of the child that is having a RecordPoll called. All other
		// children will need to have their confidence reset. If there isn't a
		// child having RecordPoll called, then the nextID will default to the
		// nil ID.
		nextID := ids.ID{}
		if len(voteStack) > 0 {
			nextID = voteStack[newStackSize-1].parentID
		}

		// If we are on the preferred branch and the nextID is the preference of
		// the snowball instance, then we are following the preferred branch.
		onPreferredBranch = onPreferredBranch && nextID.Equals(parentPreference)

		// If there wasn't an alpha threshold on the branch (either on this vote
		// or a past transitive vote), I should falter now.
		for childIDKey := range parentBlock.children {
			childID := ids.NewID(childIDKey)
			// If we don't need to transitively falter and the child is going to
			// have RecordPoll called on it, then there is no reason to reset
			// the block's confidence
			if !shouldTransitivelyFalter && childID.Equals(nextID) {
				continue
			}

			// If we finalized a child of the current block, then all other
			// children will have been rejected and removed from the tree.
			// Therefore, we need to make sure the child is still in the tree.
			childBlock, notRejected := ts.blocks[childIDKey]
			if notRejected {
				ts.ctx.Log.Verbo("Defering confidence reset of %s. Voting for %s", childID, nextID)

				// If the child is ever voted for positively, the confidence
				// must be reset first.
				childBlock.shouldFalter = true
			}
		}
	}
	return newPreferred, accepted, rejected, nil
}

// Get the preferred decendent of the provided block ID
func (ts *Topological) getPreferredDecendent(blkID ids.ID) ids.ID {
	// Traverse from the provided ID to the preferred child until there are no
	// children.
	for block := ts.blocks[blkID.Key()]; block.sb != nil; block = ts.blocks[blkID.Key()] {
		blkID = block.sb.Preference()
	}
	return blkID
}

// accept the preferred child of the provided snowman block. By accepting the
// preferred child, all other children will be rejected. When these children are
// rejected, all their descendants will be rejected.
// Returns:
//   1) The ID of the accepted vertex
//   2) The IDs of rejected vertices
func (ts *Topological) accept(n *snowmanBlock) (ids.ID, ids.Set, error) {
	// We are finalizing the block's child, so we need to get the preference
	pref := n.sb.Preference()
	ts.ctx.Log.Verbo("Accepting block with ID %s", pref)

	// Get the child and accept it
	child := n.children[pref.Key()]
	if err := child.Accept(); err != nil {
		return ids.ID{}, nil, err
	}

	// Notify anyone listening that this block was accepted.
	bytes := child.Bytes()
	ts.ctx.DecisionDispatcher.Accept(ts.ctx, pref, bytes)
	ts.ctx.ConsensusDispatcher.Accept(ts.ctx, pref, bytes)
	ts.metrics.Accepted(pref)

	// Because this is the newest accepted block, this is the new head.
	ts.head = pref

	// Because ts.blocks contains the last accepted block, we don't delete the
	// block from the blocks map here.

	rejects := make([]ids.ID, 0, len(n.children)-1)
	rejectedIDs := ids.Set{} // IDs of rejected vertices
	for childIDKey, child := range n.children {
		childID := ids.NewID(childIDKey)
		if childID.Equals(pref) {
			// don't reject the block we just accepted
			continue
		}

		if err := child.Reject(); err != nil {
			return ids.ID{}, nil, err
		}

		// Notify anyone listening that this block was rejected.
		bytes := child.Bytes()
		ts.ctx.DecisionDispatcher.Reject(ts.ctx, childID, bytes)
		ts.ctx.ConsensusDispatcher.Reject(ts.ctx, childID, bytes)
		ts.metrics.Rejected(childID)

		// Track which blocks have been directly rejected
		rejects = append(rejects, childID)
		rejectedIDs.Add(childID)
	}

	// reject all the descendants of the blocks we just rejected
	rejectedDescendants, err := ts.rejectTransitively(rejects)
	if err != nil {
		return ids.ID{}, nil, err
	}
	rejectedIDs.Union(rejectedDescendants)
	return child.ID(), rejectedIDs, nil
}

// Takes in a list of rejected ids and rejects all descendants of these IDs
// Returns the IDs of rejected vertices
func (ts *Topological) rejectTransitively(rejected []ids.ID) (ids.Set, error) {
	// the rejected array is treated as a queue, with the next element at index
	// 0 and the last element at the end of the slice.
	rejectedIDs := ids.Set{}
	for len(rejected) > 0 {
		// pop the rejected ID off the queue
		newRejectedSize := len(rejected) - 1
		rejectedID := rejected[newRejectedSize]
		rejected = rejected[:newRejectedSize]

		// get the rejected node, and remove it from the tree
		rejectedKey := rejectedID.Key()
		rejectedNode := ts.blocks[rejectedKey]
		delete(ts.blocks, rejectedKey)

		for childIDKey, child := range rejectedNode.children {
			if err := child.Reject(); err != nil {
				return nil, err
			}

			childID := ids.NewID(childIDKey)

			// Notify anyone listening that this block was rejected.
			bytes := child.Bytes()
			ts.ctx.DecisionDispatcher.Reject(ts.ctx, childID, bytes)
			ts.ctx.ConsensusDispatcher.Reject(ts.ctx, childID, bytes)
			ts.metrics.Rejected(childID)

			// add the newly rejected block to the end of the queue
			rejected = append(rejected, childID)
			rejectedIDs.Add(childID)
		}
	}
	return rejectedIDs, nil
}
