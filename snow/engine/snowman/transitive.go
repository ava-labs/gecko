// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman/poll"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/bootstrap"
	"github.com/ava-labs/avalanchego/snow/events"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

const (
	// TODO define this constant in one place rather than here and in snowman
	// Max containers size in a MultiPut message
	maxContainersLen = int(4 * network.DefaultMaxMessageSize / 5)
)

var _ Engine = &Transitive{}

// Transitive implements the Engine interface by attempting to fetch all
// transitive dependencies.
type Transitive struct {
	bootstrap.Bootstrapper
	metrics

	Params    snowball.Parameters
	Consensus snowman.Consensus

	// track outstanding preference requests
	polls poll.Set

	// blocks that have we have sent get requests for but haven't yet received
	blkReqs common.Requests

	// blocks that are queued to be issued to consensus once missing dependencies are fetched
	pending ids.Set

	// operations that are blocked on a block being issued. This could be
	// issuing another block, responding to a query, or applying votes to consensus
	blocked events.Blocker

	// number of times build block needs to be called once the number of
	// processing blocks has gone below the optimal number.
	pendingBuildBlocks int

	// errs tracks if an error has occurred in a callback
	errs wrappers.Errs
}

// Initialize implements the Engine interface
func (t *Transitive) Initialize(config Config) error {
	config.Ctx.Log.Info("initializing consensus engine")

	t.Params = config.Params
	t.Consensus = config.Consensus

	factory := poll.NewEarlyTermNoTraversalFactory(config.Params.Alpha)
	t.polls = poll.NewSet(factory,
		config.Ctx.Log,
		config.Params.Namespace,
		config.Params.Metrics,
	)

	if err := t.metrics.Initialize(config.Params.Namespace, config.Params.Metrics); err != nil {
		return err
	}

	return t.Bootstrapper.Initialize(
		config.Config,
		t.finishBootstrapping,
		fmt.Sprintf("%s_bs", config.Params.Namespace),
		config.Params.Metrics,
	)
}

// When bootstrapping is finished, this will be called.
// This initializes the consensus engine with the last accepted block.
func (t *Transitive) finishBootstrapping() error {
	lastAcceptedID, err := t.VM.LastAccepted()
	if err != nil {
		return err
	}
	lastAccepted, err := t.VM.GetBlock(lastAcceptedID)
	if err != nil {
		t.Ctx.Log.Error("failed to get last accepted block due to: %s", err)
		return err
	}

	// initialize consensus to the last accepted blockID
	if err := t.Consensus.Initialize(t.Ctx, t.Params, lastAcceptedID, lastAccepted.Height()); err != nil {
		return err
	}

	// to maintain the invariant that oracle blocks are issued in the correct
	// preferences, we need to handle the case that we are bootstrapping into an oracle block
	switch blk := lastAccepted.(type) {
	case OracleBlock:
		options, err := blk.Options()
		if err != nil {
			return err
		}
		for _, blk := range options {
			// note that deliver will set the VM's preference
			if err := t.deliver(blk); err != nil {
				return err
			}
		}
	default:
		// if there aren't blocks we need to deliver on startup, we need to set
		// the preference to the last accepted block
		if err := t.VM.SetPreference(lastAcceptedID); err != nil {
			return err
		}
	}

	t.Ctx.Log.Info("bootstrapping finished with %s as the last accepted block", lastAcceptedID)
	return nil
}

// Gossip implements the Engine interface
func (t *Transitive) Gossip() error {
	blkID, err := t.VM.LastAccepted()
	if err != nil {
		return err
	}
	blk, err := t.VM.GetBlock(blkID)
	if err != nil {
		t.Ctx.Log.Warn("dropping gossip request as %s couldn't be loaded due to %s", blkID, err)
		return nil
	}
	t.Ctx.Log.Verbo("gossiping %s as accepted to the network", blkID)
	t.Sender.Gossip(blkID, blk.Bytes())
	return nil
}

// Shutdown implements the Engine interface
func (t *Transitive) Shutdown() error {
	t.Ctx.Log.Info("shutting down consensus engine")
	return t.VM.Shutdown()
}

// Get implements the Engine interface
func (t *Transitive) Get(vdr ids.ShortID, requestID uint32, blkID ids.ID) error {
	blk, err := t.VM.GetBlock(blkID)
	if err != nil {
		// If we failed to get the block, that means either an unexpected error
		// has occurred, [vdr] is not following the protocol, or the
		// block has been pruned.
		t.Ctx.Log.Debug("Get(%s, %d, %s) failed with: %s", vdr, requestID, blkID, err)
		return nil
	}

	// Respond to the validator with the fetched block and the same requestID.
	t.Sender.Put(vdr, requestID, blkID, blk.Bytes())
	return nil
}

// GetAncestors implements the Engine interface
func (t *Transitive) GetAncestors(vdr ids.ShortID, requestID uint32, blkID ids.ID) error {
	startTime := time.Now()
	blk, err := t.VM.GetBlock(blkID)
	if err != nil { // Don't have the block. Drop this request.
		t.Ctx.Log.Verbo("couldn't get block %s. dropping GetAncestors(%s, %d, %s)", blkID, vdr, requestID, blkID)
		return nil
	}

	ancestorsBytes := make([][]byte, 1, common.MaxContainersPerMultiPut) // First elt is byte repr. of blk, then its parents, then grandparent, etc.
	ancestorsBytes[0] = blk.Bytes()
	ancestorsBytesLen := len(blk.Bytes()) + wrappers.IntLen // length, in bytes, of all elements of ancestors

	for numFetched := 1; numFetched < common.MaxContainersPerMultiPut && time.Since(startTime) < common.MaxTimeFetchingAncestors; numFetched++ {
		blk = blk.Parent()
		if blk.Status() == choices.Unknown {
			break
		}
		blkBytes := blk.Bytes()
		// Ensure response size isn't too large. Include wrappers.IntLen because the size of the message
		// is included with each container, and the size is repr. by an int.
		if newLen := wrappers.IntLen + ancestorsBytesLen + len(blkBytes); newLen < maxContainersLen {
			ancestorsBytes = append(ancestorsBytes, blkBytes)
			ancestorsBytesLen = newLen
		} else { // reached maximum response size
			break
		}
	}

	t.metrics.getAncestorsBlks.Observe(float64(len(ancestorsBytes)))
	t.Sender.MultiPut(vdr, requestID, ancestorsBytes)
	return nil
}

// Put implements the Engine interface
func (t *Transitive) Put(vdr ids.ShortID, requestID uint32, blkID ids.ID, blkBytes []byte) error {
	// bootstrapping isn't done --> we didn't send any gets --> this put is invalid
	if !t.IsBootstrapped() {
		if requestID == constants.GossipMsgRequestID {
			t.Ctx.Log.Verbo("dropping gossip Put(%s, %d, %s) due to bootstrapping",
				vdr, requestID, blkID)
		} else {
			t.Ctx.Log.Debug("dropping Put(%s, %d, %s) due to bootstrapping", vdr, requestID, blkID)
		}
		return nil
	}

	blk, err := t.VM.ParseBlock(blkBytes)
	if err != nil {
		t.Ctx.Log.Debug("failed to parse block %s: %s", blkID, err)
		t.Ctx.Log.Verbo("block:\n%s", formatting.DumpBytes{Bytes: blkBytes})
		// because GetFailed doesn't utilize the assumption that we actually
		// sent a Get message, we can safely call GetFailed here to potentially
		// abandon the request.
		return t.GetFailed(vdr, requestID)
	}

	// issue the block into consensus. If the block has already been issued,
	// this will be a noop. If this block has missing dependencies, vdr will
	// receive requests to fill the ancestry. dependencies that have already
	// been fetched, but with missing dependencies themselves won't be requested
	// from the vdr.
	if _, err := t.issueFrom(vdr, blk); err != nil {
		return err
	}
	return t.buildBlocks()
}

// GetFailed implements the Engine interface
func (t *Transitive) GetFailed(vdr ids.ShortID, requestID uint32) error {
	// not done bootstrapping --> didn't send a get --> this message is invalid
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping GetFailed(%s, %d) due to bootstrapping")
		return nil
	}

	// We don't assume that this function is called after a failed Get message.
	// Check to see if we have an outstanding request and also get what the request was for if it exists.
	blkID, ok := t.blkReqs.Remove(vdr, requestID)
	if !ok {
		t.Ctx.Log.Debug("getFailed(%s, %d) called without having sent corresponding Get", vdr, requestID)
		return nil
	}

	// Because the get request was dropped, we no longer expect blkID to be issued.
	t.blocked.Abandon(blkID)
	return t.buildBlocks()
}

// PullQuery implements the Engine interface
func (t *Transitive) PullQuery(vdr ids.ShortID, requestID uint32, blkID ids.ID) error {
	// If the engine hasn't been bootstrapped, we aren't ready to respond to queries
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping PullQuery(%s, %d, %s) due to bootstrapping", vdr, requestID, blkID)
		return nil
	}

	// Will send chits once we've issued block [blkID] into consensus
	c := &convincer{
		consensus: t.Consensus,
		sender:    t.Sender,
		vdr:       vdr,
		requestID: requestID,
		errs:      &t.errs,
	}

	// Try to issue [blkID] to consensus.
	// If we're missing an ancestor, request it from [vdr]
	added, err := t.issueFromByID(vdr, blkID)
	if err != nil {
		return err
	}

	// Wait until we've issued block [blkID] before sending chits.
	if !added {
		c.deps.Add(blkID)
	}

	t.blocked.Register(c)
	return t.buildBlocks()
}

// PushQuery implements the Engine interface
func (t *Transitive) PushQuery(vdr ids.ShortID, requestID uint32, blkID ids.ID, blkBytes []byte) error {
	// if the engine hasn't been bootstrapped, we aren't ready to respond to queries
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping PushQuery(%s, %d, %s) due to bootstrapping", vdr, requestID, blkID)
		return nil
	}

	blk, err := t.VM.ParseBlock(blkBytes)
	// If parsing fails, we just drop the request, as we didn't ask for it
	if err != nil {
		t.Ctx.Log.Debug("failed to parse block %s: %s", blkID, err)
		t.Ctx.Log.Verbo("block:\n%s", formatting.DumpBytes{Bytes: blkBytes})
		return nil
	}

	// issue the block into consensus. If the block has already been issued,
	// this will be a noop. If this block has missing dependencies, vdr will
	// receive requests to fill the ancestry. dependencies that have already
	// been fetched, but with missing dependencies themselves won't be requested
	// from the vdr.
	if _, err := t.issueFrom(vdr, blk); err != nil {
		return err
	}

	// register the chit request
	return t.PullQuery(vdr, requestID, blk.ID())
}

// Chits implements the Engine interface
func (t *Transitive) Chits(vdr ids.ShortID, requestID uint32, votes []ids.ID) error {
	// if the engine hasn't been bootstrapped, we shouldn't be receiving chits
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping Chits(%s, %d) due to bootstrapping", vdr, requestID)
		return nil
	}

	// Since this is a linear chain, there should only be one ID in the vote set
	if len(votes) != 1 {
		t.Ctx.Log.Debug("Chits(%s, %d) was called with %d votes (expected 1)", vdr, requestID, len(votes))
		// because QueryFailed doesn't utilize the assumption that we actually
		// sent a Query message, we can safely call QueryFailed here to
		// potentially abandon the request.
		return t.QueryFailed(vdr, requestID)
	}
	blkID := votes[0]

	t.Ctx.Log.Verbo("Chits(%s, %d) contains vote for %s", vdr, requestID, blkID)

	// Will record chits once [blkID] has been issued into consensus
	v := &voter{
		t:         t,
		vdr:       vdr,
		requestID: requestID,
		response:  blkID,
	}

	added, err := t.issueFromByID(vdr, blkID)
	if err != nil {
		return err
	}

	// Wait until [blkID] has been issued to consensus before for applying this chit.
	if !added {
		v.deps.Add(blkID)
	}

	t.blocked.Register(v)
	return t.buildBlocks()
}

// QueryFailed implements the Engine interface
func (t *Transitive) QueryFailed(vdr ids.ShortID, requestID uint32) error {
	// If the engine hasn't been bootstrapped, we didn't issue a query
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Warn("dropping QueryFailed(%s, %d) due to bootstrapping", vdr, requestID)
		return nil
	}

	t.blocked.Register(&voter{
		t:         t,
		vdr:       vdr,
		requestID: requestID,
	})
	return t.buildBlocks()
}

// Notify implements the Engine interface
func (t *Transitive) Notify(msg common.Message) error {
	// if the engine hasn't been bootstrapped, we shouldn't build/issue blocks from the VM
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping Notify due to bootstrapping")
		return nil
	}

	t.Ctx.Log.Verbo("snowman engine notified of %s from the vm", msg)
	switch msg {
	case common.PendingTxs:
		// the pending txs message means we should attempt to build a block.
		t.pendingBuildBlocks++
		return t.buildBlocks()
	default:
		t.Ctx.Log.Warn("unexpected message from the VM: %s", msg)
	}
	return nil
}

// Build blocks if they have been requested and the number of processing blocks
// is less than optimal.
func (t *Transitive) buildBlocks() error {
	if err := t.errs.Err; err != nil {
		return err
	}
	for t.pendingBuildBlocks > 0 && t.Consensus.NumProcessing() < t.Params.OptimalProcessing {
		t.pendingBuildBlocks--

		blk, err := t.VM.BuildBlock()
		if err != nil {
			t.Ctx.Log.Debug("VM.BuildBlock errored with: %s", err)
			return nil
		}

		// a newly created block is expected to be processing. If this check
		// fails, there is potentially an error in the VM this engine is running
		if status := blk.Status(); status != choices.Processing {
			t.Ctx.Log.Warn("attempting to issue a block with status: %s, expected Processing", status)
		}

		// The newly created block should be built on top of the preferred block.
		// Otherwise, the new block doesn't have the best chance of being confirmed.
		parentID := blk.Parent().ID()
		if pref := t.Consensus.Preference(); parentID != pref {
			t.Ctx.Log.Warn("built block with parent: %s, expected %s", parentID, pref)
		}

		added, err := t.issueWithAncestors(blk)
		if err != nil {
			return err
		}

		// issuing the block shouldn't have any missing dependencies
		if added {
			t.Ctx.Log.Verbo("successfully issued new block from the VM")
		} else {
			t.Ctx.Log.Warn("VM.BuildBlock returned a block with unissued ancestors")
		}
	}
	return nil
}

// Issue another poll to the network, asking what it prefers given the block we prefer.
// Helps move consensus along.
func (t *Transitive) repoll() {
	// if we are issuing a repoll, we should gossip our current preferences to
	// propagate the most likely branch as quickly as possible
	prefID := t.Consensus.Preference()

	for i := t.polls.Len(); i < t.Params.ConcurrentRepolls; i++ {
		t.pullQuery(prefID)
	}
}

// issueFromByID attempts to issue the branch ending with a block [blkID] into consensus.
// If we do not have [blkID], request it.
// Returns true if the block is processing in consensus or is decided.
func (t *Transitive) issueFromByID(vdr ids.ShortID, blkID ids.ID) (bool, error) {
	blk, err := t.VM.GetBlock(blkID)
	if err != nil {
		t.sendRequest(vdr, blkID)
		return false, nil
	}
	return t.issueFrom(vdr, blk)
}

// issueFrom attempts to issue the branch ending with block [blkID] to consensus.
// Returns true if the block is processing in consensus or is decided.
// If a dependency is missing, request it from [vdr].
func (t *Transitive) issueFrom(vdr ids.ShortID, blk snowman.Block) (bool, error) {
	blkID := blk.ID()
	// issue [blk] and its ancestors to consensus.
	// If the block has been decided, we don't need to issue it.
	// If the block is processing, we don't need to issue it.
	// If the block is queued to be issued, we don't need to issue it.
	for !t.Consensus.DecidedOrProcessing(blk) && !t.pending.Contains(blkID) {
		if err := t.issue(blk); err != nil {
			return false, err
		}

		blk = blk.Parent()
		blkID = blk.ID()

		// If we don't have this ancestor, request it from [vdr]
		if !blk.Status().Fetched() {
			t.sendRequest(vdr, blkID)
			return false, nil
		}
	}

	// Remove any outstanding requests for this block
	t.blkReqs.RemoveAny(blkID)

	issued := t.Consensus.DecidedOrProcessing(blk)
	if issued {
		// A dependency should never be waiting on a decided or processing
		// block. However, if the block was marked as rejected by the VM, the
		// dependencies may still be waiting. Therefore, they should abandoned.
		t.blocked.Abandon(blkID)
	}

	// Tracks performance statistics
	t.numRequests.Set(float64(t.blkReqs.Len()))
	return issued, t.errs.Err
}

// issueWithAncestors attempts to issue the branch ending with [blk] to consensus.
// Returns true if the block is processing in consensus or is decided.
// If a dependency is missing and the dependency hasn't been requested, the issuance will be abandoned.
func (t *Transitive) issueWithAncestors(blk snowman.Block) (bool, error) {
	blkID := blk.ID()
	// issue [blk] and its ancestors into consensus
	for blk.Status().Fetched() && !t.Consensus.DecidedOrProcessing(blk) && !t.pending.Contains(blkID) {
		if err := t.issue(blk); err != nil {
			return false, err
		}
		blk = blk.Parent()
		blkID = blk.ID()
	}

	// The block was issued into consensus. This is the happy path.
	if t.Consensus.DecidedOrProcessing(blk) {
		return true, nil
	}

	// There's an outstanding request for this block.
	// We can just wait for that request to succeed or fail.
	if t.blkReqs.Contains(blkID) {
		return false, nil
	}

	// We don't have this block and have no reason to expect that we will get it.
	// Abandon the block to avoid a memory leak.
	t.blocked.Abandon(blkID)
	return false, t.errs.Err
}

// Issue [blk] to consensus once its ancestors have been issued.
func (t *Transitive) issue(blk snowman.Block) error {
	blkID := blk.ID()

	// mark that the block is queued to be added to consensus once its ancestors have been
	t.pending.Add(blkID)

	// Remove any outstanding requests for this block
	t.blkReqs.RemoveAny(blkID)

	// Will add [blk] to consensus once its ancestors have been
	i := &issuer{
		t:   t,
		blk: blk,
	}

	// block on the parent if needed
	if parent := blk.Parent(); !t.Consensus.DecidedOrProcessing(parent) {
		parentID := parent.ID()
		t.Ctx.Log.Verbo("block %s waiting for parent %s to be issued", blkID, parentID)
		i.deps.Add(parentID)
	}

	t.blocked.Register(i)

	// Tracks performance statistics
	t.numRequests.Set(float64(t.blkReqs.Len()))
	t.numBlocked.Set(float64(t.pending.Len()))
	return t.errs.Err
}

// Request that [vdr] send us block [blkID]
func (t *Transitive) sendRequest(vdr ids.ShortID, blkID ids.ID) {
	// There is already an outstanding request for this block
	if t.blkReqs.Contains(blkID) {
		return
	}

	t.RequestID++
	t.blkReqs.Add(vdr, t.RequestID, blkID)
	t.Ctx.Log.Verbo("sending Get(%s, %d, %s)", vdr, t.RequestID, blkID)
	t.Sender.Get(vdr, t.RequestID, blkID)

	// Tracks performance statistics
	t.numRequests.Set(float64(t.blkReqs.Len()))
}

// send a pull query for this block ID
func (t *Transitive) pullQuery(blkID ids.ID) {
	t.Ctx.Log.Verbo("about to sample from: %s", t.Validators)
	// The validators we will query
	vdrs, err := t.Validators.Sample(t.Params.K)
	vdrBag := ids.ShortBag{}
	for _, vdr := range vdrs {
		vdrBag.Add(vdr.ID())
	}

	t.RequestID++
	if err == nil && t.polls.Add(t.RequestID, vdrBag) {
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdrBag.List()...)

		t.Sender.PullQuery(vdrSet, t.RequestID, blkID)
	} else if err != nil {
		t.Ctx.Log.Error("query for %s was dropped due to an insufficient number of validators", blkID)
	}
}

// send a push query for this block
func (t *Transitive) pushQuery(blk snowman.Block) {
	t.Ctx.Log.Verbo("about to sample from: %s", t.Validators)
	vdrs, err := t.Validators.Sample(t.Params.K)
	vdrBag := ids.ShortBag{}
	for _, vdr := range vdrs {
		vdrBag.Add(vdr.ID())
	}

	t.RequestID++
	if err == nil && t.polls.Add(t.RequestID, vdrBag) {
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdrBag.List()...)

		t.Sender.PushQuery(vdrSet, t.RequestID, blk.ID(), blk.Bytes())
	} else if err != nil {
		t.Ctx.Log.Error("query for %s was dropped due to an insufficient number of validators", blk.ID())
	}
}

// issue [blk] to consensus
func (t *Transitive) deliver(blk snowman.Block) error {
	if t.Consensus.DecidedOrProcessing(blk) {
		return nil
	}

	// we are no longer waiting on adding the block to consensus, so it is no
	// longer pending
	blkID := blk.ID()
	t.pending.Remove(blkID)

	if !t.Consensus.AcceptedOrProcessing(blk.Parent()) {
		// if the parent isn't processing or the last accepted block, then this
		// block is effectively rejected
		t.blocked.Abandon(blkID)
		t.numBlocked.Set(float64(t.pending.Len())) // Tracks performance statistics
		return t.errs.Err
	}

	// By ensuring that the parent is either processing or accepted, it is
	// guaranteed that the parent was successfully verified. This means that
	// calling Verify on this block is allowed.

	// make sure this block is valid
	if err := blk.Verify(); err != nil {
		t.Ctx.Log.Debug("block failed verification due to %s, dropping block", err)

		// if verify fails, then all descendants are also invalid
		t.blocked.Abandon(blkID)
		t.numBlocked.Set(float64(t.pending.Len())) // Tracks performance statistics
		return t.errs.Err
	}

	t.Ctx.Log.Verbo("adding block to consensus: %s", blkID)
	if err := t.Consensus.Add(blk); err != nil {
		return err
	}

	// Add all the oracle blocks if they exist. We call verify on all the blocks
	// and add them to consensus before marking anything as fulfilled to avoid
	// any potential reentrant bugs.
	added := []snowman.Block{}
	dropped := []snowman.Block{}
	if blk, ok := blk.(OracleBlock); ok {
		options, err := blk.Options()
		if err != nil {
			return err
		}
		for _, blk := range options {
			if err := blk.Verify(); err != nil {
				t.Ctx.Log.Debug("block failed verification due to %s, dropping block", err)
				dropped = append(dropped, blk)
			} else {
				if err := t.Consensus.Add(blk); err != nil {
					return err
				}
				added = append(added, blk)
			}
		}
	}

	if err := t.VM.SetPreference(t.Consensus.Preference()); err != nil {
		return err
	}

	// If the block is now preferred, query the network for its preferences
	// with this new block.
	if t.Consensus.IsPreferred(blk) {
		t.pushQuery(blk)
	}

	t.blocked.Fulfill(blkID)
	for _, blk := range added {
		if t.Consensus.IsPreferred(blk) {
			t.pushQuery(blk)
		}

		blkID := blk.ID()
		t.pending.Remove(blkID)
		t.blocked.Fulfill(blkID)
		t.blkReqs.RemoveAny(blkID)
	}
	for _, blk := range dropped {
		blkID := blk.ID()
		t.pending.Remove(blkID)
		t.blocked.Abandon(blkID)
		t.blkReqs.RemoveAny(blkID)
	}

	// If we should issue multiple queries at the same time, we need to repoll
	t.repoll()

	// Tracks performance statistics
	t.numRequests.Set(float64(t.blkReqs.Len()))
	t.numBlocked.Set(float64(t.pending.Len()))
	return t.errs.Err
}

// IsBootstrapped returns true iff this chain is done bootstrapping
func (t *Transitive) IsBootstrapped() bool {
	return t.Ctx.IsBootstrapped()
}

// Health implements the common.Engine interface
func (t *Transitive) HealthCheck() (interface{}, error) {
	var (
		consensusIntf interface{} = struct{}{}
		consensusErr  error
	)
	if t.Ctx.IsBootstrapped() {
		consensusIntf, consensusErr = t.Consensus.HealthCheck()
	}
	vmIntf, vmErr := t.VM.HealthCheck()
	intf := map[string]interface{}{
		"consensus": consensusIntf,
		"vm":        vmIntf,
	}
	if consensusErr == nil {
		return intf, vmErr
	}
	if vmErr == nil {
		return intf, consensusErr
	}
	return intf, fmt.Errorf("vm: %s ; consensus: %s", vmErr, consensusErr)
}

// GetBlock implements the snowman.Engine interface
func (t *Transitive) GetBlock(blkID ids.ID) (snowman.Block, error) {
	return t.VM.GetBlock(blkID)
}

// GetVM implements the snowman.Engine interface
func (t *Transitive) GetVM() common.VM {
	return t.VM
}
