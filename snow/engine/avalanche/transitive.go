// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/network"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche/poll"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/bootstrap"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/events"
	"github.com/ava-labs/avalanchego/utils/constants"
	"github.com/ava-labs/avalanchego/utils/formatting"
	"github.com/ava-labs/avalanchego/utils/sampler"
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

	Params    avalanche.Parameters
	Consensus avalanche.Consensus

	polls poll.Set // track people I have asked for their preference

	// The set of vertices that have been requested in Get messages but not yet received
	outstandingVtxReqs common.Requests

	// missingTxs tracks transaction that are missing
	missingTxs ids.Set

	// IDs of vertices that are queued to be added to consensus but haven't yet been
	// because of missing dependencies
	pending ids.Set

	// vtxBlocked tracks operations that are blocked on vertices
	// txBlocked tracks operations that are blocked on transactions
	vtxBlocked, txBlocked events.Blocker

	// transactions that have been provided from the VM but that are pending to
	// be issued once the number of processing vertices has gone below the
	// optimal number.
	pendingTxs []snowstorm.Tx

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

func (t *Transitive) finishBootstrapping() error {
	// Load the vertices that were last saved as the accepted frontier
	edge := t.Manager.Edge()
	frontier := make([]avalanche.Vertex, 0, len(edge))
	for _, vtxID := range edge {
		if vtx, err := t.Manager.GetVtx(vtxID); err == nil {
			frontier = append(frontier, vtx)
		} else {
			t.Ctx.Log.Error("vertex %s failed to be loaded from the frontier with %s", vtxID, err)
		}
	}

	t.Ctx.Log.Info("bootstrapping finished with %d vertices in the accepted frontier", len(frontier))
	return t.Consensus.Initialize(t.Ctx, t.Params, frontier)
}

// Gossip implements the Engine interface
func (t *Transitive) Gossip() error {
	edge := t.Manager.Edge()
	if len(edge) == 0 {
		t.Ctx.Log.Verbo("dropping gossip request as no vertices have been accepted")
		return nil
	}

	s := sampler.NewUniform()
	if err := s.Initialize(uint64(len(edge))); err != nil {
		return err // Should never really happen
	}
	indices, err := s.Sample(1)
	if err != nil {
		return err // Also should never really happen because the edge has positive length
	}
	vtxID := edge[int(indices[0])]
	vtx, err := t.Manager.GetVtx(vtxID)
	if err != nil {
		t.Ctx.Log.Warn("dropping gossip request as %s couldn't be loaded due to: %s", vtxID, err)
		return nil
	}

	t.Ctx.Log.Verbo("gossiping %s as accepted to the network", vtxID)
	t.Sender.Gossip(vtxID, vtx.Bytes())
	return nil
}

// Shutdown implements the Engine interface
func (t *Transitive) Shutdown() error {
	t.Ctx.Log.Info("shutting down consensus engine")
	return t.VM.Shutdown()
}

// Get implements the Engine interface
func (t *Transitive) Get(vdr ids.ShortID, requestID uint32, vtxID ids.ID) error {
	// If this engine has access to the requested vertex, provide it
	if vtx, err := t.Manager.GetVtx(vtxID); err == nil {
		t.Sender.Put(vdr, requestID, vtxID, vtx.Bytes())
	}
	return nil
}

// GetAncestors implements the Engine interface
func (t *Transitive) GetAncestors(vdr ids.ShortID, requestID uint32, vtxID ids.ID) error {
	startTime := time.Now()
	t.Ctx.Log.Verbo("GetAncestors(%s, %d, %s) called", vdr, requestID, vtxID)
	vertex, err := t.Manager.GetVtx(vtxID)
	if err != nil || vertex.Status() == choices.Unknown {
		t.Ctx.Log.Verbo("dropping getAncestors")
		return nil // Don't have the requested vertex. Drop message.
	}

	queue := make([]avalanche.Vertex, 1, common.MaxContainersPerMultiPut) // for BFS
	queue[0] = vertex
	ancestorsBytesLen := 0                                               // length, in bytes, of vertex and its ancestors
	ancestorsBytes := make([][]byte, 0, common.MaxContainersPerMultiPut) // vertex and its ancestors in BFS order
	visited := ids.Set{}                                                 // IDs of vertices that have been in queue before
	visited.Add(vertex.ID())

	for len(ancestorsBytes) < common.MaxContainersPerMultiPut && len(queue) > 0 && time.Since(startTime) < common.MaxTimeFetchingAncestors {
		var vtx avalanche.Vertex
		vtx, queue = queue[0], queue[1:] // pop
		vtxBytes := vtx.Bytes()
		// Ensure response size isn't too large. Include wrappers.IntLen because the size of the message
		// is included with each container, and the size is repr. by an int.
		if newLen := wrappers.IntLen + ancestorsBytesLen + len(vtxBytes); newLen < maxContainersLen {
			ancestorsBytes = append(ancestorsBytes, vtxBytes)
			ancestorsBytesLen = newLen
		} else { // reached maximum response size
			break
		}
		parents, err := vtx.Parents()
		if err != nil {
			return err
		}
		for _, parent := range parents {
			if parent.Status() == choices.Unknown { // Don't have this vertex;ignore
				continue
			}
			if parentID := parent.ID(); !visited.Contains(parentID) { // If already visited, ignore
				queue = append(queue, parent)
				visited.Add(parentID)
			}
		}
	}

	t.metrics.getAncestorsVtxs.Observe(float64(len(ancestorsBytes)))
	t.Sender.MultiPut(vdr, requestID, ancestorsBytes)
	return nil
}

// Put implements the Engine interface
func (t *Transitive) Put(vdr ids.ShortID, requestID uint32, vtxID ids.ID, vtxBytes []byte) error {
	t.Ctx.Log.Verbo("Put(%s, %d, %s) called", vdr, requestID, vtxID)

	if !t.Ctx.IsBootstrapped() { // Bootstrapping unfinished --> didn't call Get --> this message is invalid
		if requestID == constants.GossipMsgRequestID {
			t.Ctx.Log.Verbo("dropping gossip Put(%s, %d, %s) due to bootstrapping", vdr, requestID, vtxID)
		} else {
			t.Ctx.Log.Debug("dropping Put(%s, %d, %s) due to bootstrapping", vdr, requestID, vtxID)
		}
		return nil
	}

	vtx, err := t.Manager.ParseVtx(vtxBytes)
	if err != nil {
		t.Ctx.Log.Debug("failed to parse vertex %s due to: %s", vtxID, err)
		t.Ctx.Log.Verbo("vertex:\n%s", formatting.DumpBytes{Bytes: vtxBytes})
		return t.GetFailed(vdr, requestID)
	}
	if _, err := t.issueFrom(vdr, vtx); err != nil {
		return err
	}
	return t.attemptToIssueTxs()
}

// GetFailed implements the Engine interface
func (t *Transitive) GetFailed(vdr ids.ShortID, requestID uint32) error {
	if !t.Ctx.IsBootstrapped() { // Bootstrapping unfinished --> didn't call Get --> this message is invalid
		t.Ctx.Log.Debug("dropping GetFailed(%s, %d) due to bootstrapping", vdr, requestID)
		return nil
	}

	vtxID, ok := t.outstandingVtxReqs.Remove(vdr, requestID)
	if !ok {
		t.Ctx.Log.Debug("GetFailed(%s, %d) called without having sent corresponding Get", vdr, requestID)
		return nil
	}

	t.vtxBlocked.Abandon(vtxID)

	if t.outstandingVtxReqs.Len() == 0 {
		for txID := range t.missingTxs {
			t.txBlocked.Abandon(txID)
		}
		t.missingTxs.Clear()
	}

	// Track performance statistics
	t.numVtxRequests.Set(float64(t.outstandingVtxReqs.Len()))
	t.numMissingTxs.Set(float64(t.missingTxs.Len()))
	return t.attemptToIssueTxs()
}

// PullQuery implements the Engine interface
func (t *Transitive) PullQuery(vdr ids.ShortID, requestID uint32, vtxID ids.ID) error {
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping PullQuery(%s, %d, %s) due to bootstrapping",
			vdr, requestID, vtxID)
		return nil
	}

	// Will send chits to [vdr] once we have [vtxID] and its dependencies
	c := &convincer{
		consensus: t.Consensus,
		sender:    t.Sender,
		vdr:       vdr,
		requestID: requestID,
		errs:      &t.errs,
	}

	// If we have [vtxID], put it into consensus if we haven't already.
	// If not, fetch it.
	inConsensus, err := t.issueFromByID(vdr, vtxID)
	if err != nil {
		return err
	}

	// [vtxID] isn't in consensus yet because we don't have it or a dependency.
	if !inConsensus {
		c.deps.Add(vtxID) // Don't send chits until [vtxID] is in consensus.
	}

	// Wait until [vtxID] and its dependencies have been added to consensus before sending chits
	t.vtxBlocked.Register(c)
	return t.attemptToIssueTxs()
}

// PushQuery implements the Engine interface
func (t *Transitive) PushQuery(vdr ids.ShortID, requestID uint32, vtxID ids.ID, vtxBytes []byte) error {
	if !t.Ctx.IsBootstrapped() {
		// We're bootstrapping, so ignore this query.
		t.Ctx.Log.Debug("dropping PushQuery(%s, %d, %s) due to bootstrapping", vdr, requestID, vtxID)
		return nil
	}

	vtx, err := t.Manager.ParseVtx(vtxBytes)
	if err != nil {
		t.Ctx.Log.Debug("failed to parse vertex %s due to: %s", vtxID, err)
		t.Ctx.Log.Verbo("vertex:\n%s", formatting.DumpBytes{Bytes: vtxBytes})
		return nil
	}

	if _, err := t.issueFrom(vdr, vtx); err != nil {
		return err
	}

	return t.PullQuery(vdr, requestID, vtx.ID())
}

// Chits implements the Engine interface
func (t *Transitive) Chits(vdr ids.ShortID, requestID uint32, votes []ids.ID) error {
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping Chits(%s, %d) due to bootstrapping", vdr, requestID)
		return nil
	}

	v := &voter{
		t:         t,
		vdr:       vdr,
		requestID: requestID,
		response:  votes,
	}
	for _, vote := range votes {
		if added, err := t.issueFromByID(vdr, vote); err != nil {
			return err
		} else if !added {
			v.deps.Add(vote)
		}
	}

	t.vtxBlocked.Register(v)
	return t.attemptToIssueTxs()
}

// QueryFailed implements the Engine interface
func (t *Transitive) QueryFailed(vdr ids.ShortID, requestID uint32) error {
	return t.Chits(vdr, requestID, nil)
}

// Notify implements the Engine interface
func (t *Transitive) Notify(msg common.Message) error {
	if !t.Ctx.IsBootstrapped() {
		t.Ctx.Log.Debug("dropping Notify due to bootstrapping")
		return nil
	}

	switch msg {
	case common.PendingTxs:
		t.pendingTxs = append(t.pendingTxs, t.VM.PendingTxs()...)
		return t.attemptToIssueTxs()
	default:
		t.Ctx.Log.Warn("unexpected message from the VM: %s", msg)
	}
	return nil
}

func (t *Transitive) attemptToIssueTxs() error {
	err := t.errs.Err
	if err != nil {
		return err
	}

	t.pendingTxs, err = t.batch(t.pendingTxs, false /*=force*/, false /*=empty*/, true /*=limit*/)
	return err
}

// If there are pending transactions from the VM, issue them.
// If we're not already at the limit for number of concurrent polls, issue a new
// query.
func (t *Transitive) repoll() {
	for i := t.polls.Len(); i < t.Params.ConcurrentRepolls && !t.errs.Errored(); i++ {
		t.issueRepoll()
	}
}

// issueFromByID issues the branch ending with vertex [vtxID] to consensus.
// Fetches [vtxID] if we don't have it locally.
// Returns true if [vtx] has been added to consensus (now or previously)
func (t *Transitive) issueFromByID(vdr ids.ShortID, vtxID ids.ID) (bool, error) {
	vtx, err := t.Manager.GetVtx(vtxID)
	if err != nil {
		// We don't have [vtxID]. Request it.
		t.sendRequest(vdr, vtxID)
		return false, nil
	}
	return t.issueFrom(vdr, vtx)
}

// issueFrom issues the branch ending with [vtx] to consensus.
// Assumes we have [vtx] locally
// Returns true if [vtx] has been added to consensus (now or previously)
func (t *Transitive) issueFrom(vdr ids.ShortID, vtx avalanche.Vertex) (bool, error) {
	issued := true
	// Before we issue [vtx] into consensus, we have to issue its ancestors.
	// Go through [vtx] and its ancestors. issue each ancestor that hasn't yet been issued.
	// If we find a missing ancestor, fetch it and note that we can't issue [vtx] yet.
	ancestry := vertex.NewHeap()
	ancestry.Push(vtx)
	for ancestry.Len() > 0 {
		vtx := ancestry.Pop()

		if t.Consensus.VertexIssued(vtx) {
			// This vertex has been issued --> its ancestors have been issued.
			// No need to try to issue it or its ancestors
			continue
		}
		if t.pending.Contains(vtx.ID()) {
			issued = false
			continue
		}

		parents, err := vtx.Parents()
		if err != nil {
			return false, err
		}
		// Ensure we have ancestors of this vertex
		for _, parent := range parents {
			if !parent.Status().Fetched() {
				// We don't have the parent. Request it.
				t.sendRequest(vdr, parent.ID())
				// We're missing an ancestor so we can't have issued the vtx in this method's argument
				issued = false
			} else {
				// Come back to this vertex later to make sure it and its ancestors have been fetched/issued
				ancestry.Push(parent)
			}
		}

		// Queue up this vertex to be issued once its dependencies are met
		if err := t.issue(vtx); err != nil {
			return false, err
		}
	}
	return issued, nil
}

// issue queues [vtx] to be put into consensus after its dependencies are met.
// Assumes we have [vtx].
func (t *Transitive) issue(vtx avalanche.Vertex) error {
	vtxID := vtx.ID()

	// Add to set of vertices that have been queued up to be issued but haven't been yet
	t.pending.Add(vtxID)
	t.outstandingVtxReqs.RemoveAny(vtxID)

	// Will put [vtx] into consensus once dependencies are met
	i := &issuer{
		t:   t,
		vtx: vtx,
	}

	parents, err := vtx.Parents()
	if err != nil {
		return err
	}
	for _, parent := range parents {
		if !t.Consensus.VertexIssued(parent) {
			// This parent hasn't been issued yet. Add it as a dependency.
			i.vtxDeps.Add(parent.ID())
		}
	}

	txs, err := vtx.Txs()
	if err != nil {
		return err
	}
	txIDs := ids.Set{}
	for _, tx := range txs {
		txIDs.Add(tx.ID())
	}

	for _, tx := range txs {
		for _, dep := range tx.Dependencies() {
			depID := dep.ID()
			if !txIDs.Contains(depID) && !t.Consensus.TxIssued(dep) {
				// This transaction hasn't been issued yet. Add it as a dependency.
				t.missingTxs.Add(depID)
				i.txDeps.Add(depID)
			}
		}
	}

	t.Ctx.Log.Verbo("vertex %s is blocking on %d vertices and %d transactions",
		vtxID, i.vtxDeps.Len(), i.txDeps.Len())

	// Wait until all the parents of [vtx] are added to consensus before adding [vtx]
	t.vtxBlocked.Register(&vtxIssuer{i: i})
	// Wait until all the parents of [tx] are added to consensus before adding [vtx]
	t.txBlocked.Register(&txIssuer{i: i})

	if t.outstandingVtxReqs.Len() == 0 {
		// There are no outstanding vertex requests but we don't have these transactions, so we're not getting them.
		for txID := range t.missingTxs {
			t.txBlocked.Abandon(txID)
		}
		t.missingTxs.Clear()
	}

	// Track performance statistics
	t.numVtxRequests.Set(float64(t.outstandingVtxReqs.Len()))
	t.numMissingTxs.Set(float64(t.missingTxs.Len()))
	t.numPendingVts.Set(float64(t.pending.Len()))
	return t.errs.Err
}

// Batchs [txs] into vertices and issue them.
// If [force] is true, forces each tx to be issued.
// Otherwise, some txs may not be put into vertices that are issued.
// If [empty], will always result in a new poll.
func (t *Transitive) batch(txs []snowstorm.Tx, force, empty, limit bool) ([]snowstorm.Tx, error) {
	if limit && t.Params.OptimalProcessing <= t.Consensus.NumProcessing() {
		return txs, nil
	}
	issuedTxs := ids.Set{}
	consumed := ids.Set{}
	issued := false
	orphans := t.Consensus.Orphans()
	start := 0
	end := 0
	for end < len(txs) {
		tx := txs[end]
		inputs := ids.Set{}
		inputs.Add(tx.InputIDs()...)
		overlaps := consumed.Overlaps(inputs)
		if end-start >= t.Params.BatchSize || (force && overlaps) {
			if err := t.issueBatch(txs[start:end]); err != nil {
				return nil, err
			}
			if limit && t.Params.OptimalProcessing <= t.Consensus.NumProcessing() {
				return txs[end:], nil
			}
			start = end
			consumed.Clear()
			issued = true
			overlaps = false
		}

		if txID := tx.ID(); !overlaps && // should never allow conflicting txs in the same vertex
			!issuedTxs.Contains(txID) && // shouldn't issue duplicated transactions to the same vertex
			(force || t.Consensus.IsVirtuous(tx)) && // force allows for a conflict to be issued
			(!t.Consensus.TxIssued(tx) || orphans.Contains(txID)) { // should only reissue orphaned txs
			end++
			issuedTxs.Add(txID)
			consumed.Union(inputs)
		} else {
			newLen := len(txs) - 1
			txs[end] = txs[newLen]
			txs[newLen] = nil
			txs = txs[:newLen]
		}
	}

	if end > start {
		return txs[end:], t.issueBatch(txs[start:end])
	}
	if empty && !issued {
		t.issueRepoll()
	}
	return txs[end:], nil
}

// Issues a new poll for a preferred vertex in order to move consensus along
func (t *Transitive) issueRepoll() {
	preferredIDs := t.Consensus.Preferences()
	if preferredIDs.Len() == 0 {
		t.Ctx.Log.Error("re-query attempt was dropped due to no pending vertices")
		return
	}

	vtxID := preferredIDs.CappedList(1)[0]
	vdrs, err := t.Validators.Sample(t.Params.K) // Validators to sample
	vdrBag := ids.ShortBag{}                     // IDs of validators to be sampled
	for _, vdr := range vdrs {
		vdrBag.Add(vdr.ID())
	}

	vdrSet := ids.ShortSet{}
	vdrSet.Add(vdrBag.List()...)

	// Poll the network
	t.RequestID++
	if err == nil && t.polls.Add(t.RequestID, vdrBag) {
		t.Sender.PullQuery(vdrSet, t.RequestID, vtxID)
	} else if err != nil {
		t.Ctx.Log.Error("re-query for %s was dropped due to an insufficient number of validators", vtxID)
	}
}

// Puts a batch of transactions into a vertex and issues it into consensus.
func (t *Transitive) issueBatch(txs []snowstorm.Tx) error {
	t.Ctx.Log.Verbo("batching %d transactions into a new vertex", len(txs))

	// Randomly select parents of this vertex from among the virtuous set
	virtuousIDs := t.Consensus.Virtuous().CappedList(t.Params.Parents)
	numVirtuousIDs := len(virtuousIDs)
	s := sampler.NewUniform()
	if err := s.Initialize(uint64(numVirtuousIDs)); err != nil {
		return err
	}

	indices, err := s.Sample(numVirtuousIDs)
	if err != nil {
		return err
	}

	parentIDs := make([]ids.ID, len(indices))
	for i, index := range indices {
		parentIDs[i] = virtuousIDs[int(index)]
	}

	vtx, err := t.Manager.BuildVtx(0, parentIDs, txs, nil)
	if err != nil {
		t.Ctx.Log.Warn("error building new vertex with %d parents and %d transactions",
			len(parentIDs), len(txs))
		return nil
	}
	return t.issue(vtx)
}

// Send a request to [vdr] asking them to send us vertex [vtxID]
func (t *Transitive) sendRequest(vdr ids.ShortID, vtxID ids.ID) {
	if t.outstandingVtxReqs.Contains(vtxID) {
		t.Ctx.Log.Debug("not sending request for vertex %s because there is already an outstanding request for it", vtxID)
		return
	}
	t.RequestID++
	t.outstandingVtxReqs.Add(vdr, t.RequestID, vtxID) // Mark that there is an outstanding request for this vertex
	t.Sender.Get(vdr, t.RequestID, vtxID)
	t.numVtxRequests.Set(float64(t.outstandingVtxReqs.Len())) // Tracks performance statistics
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

// GetVtx returns a vertex by its ID.
// Returns database.ErrNotFound if unknown.
func (t *Transitive) GetVtx(vtxID ids.ID) (avalanche.Vertex, error) {
	return t.Manager.GetVtx(vtxID)
}

func (t *Transitive) GetVM() common.VM {
	return t.VM
}
