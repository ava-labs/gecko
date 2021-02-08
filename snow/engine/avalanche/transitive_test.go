// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/avalanche"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"
	"github.com/ava-labs/avalanchego/snow/engine/avalanche/vertex"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils"
	"github.com/ava-labs/avalanchego/utils/wrappers"
)

var (
	errUnknownVertex = errors.New("unknown vertex")
	errFailedParsing = errors.New("failed parsing")
	errMissing       = errors.New("missing")
)

func TestEngineShutdown(t *testing.T) {
	config := DefaultConfig()
	vmShutdownCalled := false
	vm := &vertex.TestVM{}
	vm.ShutdownF = func() error { vmShutdownCalled = true; return nil }
	config.VM = vm

	transitive := &Transitive{}

	if err := transitive.Initialize(config); err != nil {
		t.Fatal(err)
	}
	if err := transitive.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if !vmShutdownCalled {
		t.Fatal("Shutting down the Transitive did not shutdown the VM")
	}
}

func TestEngineAdd(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	manager.CantEdge = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if te.Ctx.ChainID != ids.Empty {
		t.Fatalf("Wrong chain ID")
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{
			&avalanche.TestVertex{TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Unknown,
			}},
		},
		BytesV: []byte{1},
	}

	asked := new(bool)
	reqID := new(uint32)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		if vdr != inVdr {
			t.Fatalf("Asking wrong validator for vertex")
		}
		if vtx.ParentsV[0].ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, vtx.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return vtx, nil
	}

	if err := te.Put(vdr, 0, vtx.ID(), vtx.Bytes()); err != nil {
		t.Fatal(err)
	}

	manager.ParseF = nil

	if !*asked {
		t.Fatalf("Didn't ask for a missing vertex")
	}

	if len(te.vtxBlocked) != 1 {
		t.Fatalf("Should have been blocking on request")
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) { return nil, errFailedParsing }

	if err := te.Put(vdr, *reqID, vtx.ParentsV[0].ID(), nil); err != nil {
		t.Fatal(err)
	}

	manager.ParseF = nil

	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Should have finished blocking issue")
	}
}

func TestEngineFulfillMissingDependencies(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	manager.CantEdge = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if te.Ctx.ChainID != ids.Empty {
		t.Fatalf("Wrong chain ID")
	}

	trA := &conflicts.TestTransition{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Unknown,
	}
	txA := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trA,
	}
	// [trB] depends on [trA]
	trB := &conflicts.TestTransition{
		IDV:           ids.GenerateTestID(),
		StatusV:       choices.Processing,
		DependenciesV: []conflicts.Transition{trA},
	}
	txB := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trB,
	}

	parentVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		BytesV: []byte{1},
		TxsV:   []conflicts.Tx{txA},
	}
	childVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{
			parentVtx,
		},
		BytesV: []byte{2},
		TxsV:   []conflicts.Tx{txB},
	}

	asked := new(bool)
	reqID := new(uint32)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		if vdr != inVdr {
			t.Fatalf("Asking wrong validator for vertex")
		}
		if parentVtx.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, childVtx.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return childVtx, nil
	}

	if err := te.Put(vdr, 0, childVtx.ID(), childVtx.Bytes()); err != nil {
		t.Fatal(err)
	}

	if !*asked {
		t.Fatalf("Didn't ask for a missing vertex")
	}
	if len(te.vtxBlocked) != 1 {
		t.Fatalf("Should have been blocking on 1 missing vertex dependency")
	}
	if len(te.trBlocked) != 1 {
		t.Fatalf("Should have been blocking on 1 missing transition dependency")
	}
	missing, ok := te.missingTransitions[txA.Epoch()]
	if !ok || missing.Len() != 1 || len(te.missingTransitions) > 1 {
		t.Fatalf("Should have had one missing transition")
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, parentVtx.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return parentVtx, nil
	}

	pushedParentVtx := new(bool)
	pushedChildVtx := new(bool)

	// it should send two push queries first with parent vtx and then child vertex
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, vtx []byte) {
		if inVdrs.Len() != 1 {
			t.Fatalf("Expected push query to be sent to one validator")
		} else if !inVdrs.Contains(vdr) {
			t.Fatalf("Expected to push query validator")
		}

		switch {
		case !*pushedParentVtx:
			if vtxID != parentVtx.ID() || !bytes.Equal(vtx, parentVtx.Bytes()) {
				t.Fatalf("Expected to push query parent vertex")
			}
			*pushedParentVtx = true
		case !*pushedChildVtx:
			if vtxID != childVtx.ID() || !bytes.Equal(vtx, childVtx.Bytes()) {
				t.Fatalf("Expected to push child vertex")
			}
			*pushedChildVtx = true
		default:
			t.Fatalf("Called push query more times than expected")
		}
	}

	if err := te.Put(vdr, *reqID, parentVtx.ID(), parentVtx.Bytes()); err != nil {
		t.Fatal(err)
	}

	if !*pushedParentVtx {
		t.Fatalf("Expected to push query parent vertex")
	}
	if !*pushedChildVtx {
		t.Fatalf("Expected to push query child vertex")
	}
	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Expected 0 blocked vertices, but found %d blocked vertices", len(te.vtxBlocked))
	}
	if len(te.trBlocked) != 0 {
		t.Fatalf("Expected 0 blocked transitions, but found %d blocked transitions", len(te.trBlocked))
	}
	t.Log(te.missingTransitions)
	if len(te.missingTransitions) != 0 {
		t.Fatalf("Expected 0 missing transitions, but found %d missing transitions", len(te.missingTransitions))
	}
}

func TestEngineQuery(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
		BytesV:   []byte{0, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}

		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vertexed := new(bool)
	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if *vertexed {
			t.Fatalf("Sent multiple requests")
		}
		*vertexed = true
		if vtxID != vtx0.ID() {
			t.Fatalf("Wrong vertex requested")
		}
		return nil, errUnknownVertex
	}

	asked := new(bool)
	sender.GetF = func(inVdr ids.ShortID, _ uint32, vtxID ids.ID) {
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		if vdr != inVdr {
			t.Fatalf("Asking wrong validator for vertex")
		}
		if vtx0.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	if err := te.PullQuery(vdr, 0, vtx0.ID()); err != nil {
		t.Fatal(err)
	}

	if !*vertexed {
		t.Fatalf("Didn't request vertex")
	}
	if !*asked {
		t.Fatalf("Didn't request vertex from validator")
	}

	queried := new(bool)
	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, vtx []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if vtx0.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	chitted := new(bool)
	sender.ChitsF = func(inVdr ids.ShortID, _ uint32, prefs []ids.ID) {
		if *chitted {
			t.Fatalf("Sent multiple chits")
		}
		*chitted = true
		if len(prefs) != 1 || prefs[0] != vtx0.ID() {
			t.Fatalf("Wrong chits preferences")
		}
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, vtx0.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return vtx0, nil
	}
	if err := te.Put(vdr, 0, vtx0.ID(), vtx0.Bytes()); err != nil {
		t.Fatal(err)
	}
	manager.ParseF = nil

	if !*queried {
		t.Fatalf("Didn't ask for preferences")
	}
	if !*chitted {
		t.Fatalf("Didn't provide preferences")
	}

	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
		BytesV:   []byte{5, 4, 3, 2, 1, 9},
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == vtx0.ID() {
			return &avalanche.TestVertex{
				TestDecidable: choices.TestDecidable{
					StatusV: choices.Unknown,
				},
			}, nil
		}
		if vtxID == vtx1.ID() {
			return nil, errUnknownVertex
		}
		t.Fatalf("Wrong vertex requested")
		panic("Should have failed")
	}

	*asked = false
	sender.GetF = func(inVdr ids.ShortID, _ uint32, vtxID ids.ID) {
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		if vdr != inVdr {
			t.Fatalf("Asking wrong validator for vertex")
		}
		if vtx1.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	if err := te.Chits(vdr, *queryRequestID, []ids.ID{vtx1.ID()}); err != nil {
		t.Fatal(err)
	}

	*queried = false
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, vtx []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if vtx1.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, vtx1.Bytes()) {
			t.Fatalf("Wrong bytes")
		}

		manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
			if vtxID == vtx0.ID() {
				return &avalanche.TestVertex{
					TestDecidable: choices.TestDecidable{
						StatusV: choices.Processing,
					},
				}, nil
			}
			if vtxID == vtx1.ID() {
				return vtx1, nil
			}
			t.Fatalf("Wrong vertex requested")
			panic("Should have failed")
		}

		return vtx1, nil
	}
	if err := te.Put(vdr, 0, vtx1.ID(), vtx1.Bytes()); err != nil {
		t.Fatal(err)
	}
	manager.ParseF = nil

	if vtx0.Status() != choices.Accepted {
		t.Fatalf("Should have executed vertex")
	}
	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Should have finished blocking")
	}

	_ = te.polls.String() // Shouldn't panic

	if err := te.QueryFailed(vdr, *queryRequestID); err != nil {
		t.Fatal(err)
	}
	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Should have finished blocking")
	}
}

func TestEngineMultipleQuery(t *testing.T) {
	config := DefaultConfig()

	config.Params = avalanche.Parameters{
		Parameters: snowball.Parameters{
			Metrics:           prometheus.NewRegistry(),
			K:                 3,
			Alpha:             2,
			BetaVirtuous:      1,
			BetaRogue:         2,
			ConcurrentRepolls: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}

	vals := validators.NewSet()
	config.Validators = vals

	vdr0 := ids.GenerateTestShortID()
	vdr1 := ids.GenerateTestShortID()
	vdr2 := ids.GenerateTestShortID()

	errs := wrappers.Errs{}
	errs.Add(
		vals.AddWeight(vdr0, 1),
		vals.AddWeight(vdr1, 1),
		vals.AddWeight(vdr2, 1),
	)
	if errs.Errored() {
		t.Fatal(errs.Err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	queried := new(bool)
	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, vtx []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr0, vdr1, vdr2)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if vtx0.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	if err := te.issue(vtx0, false); err != nil {
		t.Fatal(err)
	}

	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
	}

	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		case vtx0.ID():
			return vtx0, nil
		case vtx1.ID():
			return nil, errUnknownVertex
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	asked := new(bool)
	reqID := new(uint32)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		if vdr0 != inVdr {
			t.Fatalf("Asking wrong validator for vertex")
		}
		if vtx1.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	s0 := []ids.ID{vtx0.ID(), vtx1.ID()}

	s2 := []ids.ID{vtx0.ID()}

	if err := te.Chits(vdr0, *queryRequestID, s0); err != nil {
		t.Fatal(err)
	}
	if err := te.QueryFailed(vdr1, *queryRequestID); err != nil {
		t.Fatal(err)
	}
	if err := te.Chits(vdr2, *queryRequestID, s2); err != nil {
		t.Fatal(err)
	}

	// Should be dropped because the query was marked as failed
	if err := te.Chits(vdr1, *queryRequestID, s0); err != nil {
		t.Fatal(err)
	}

	if err := te.GetFailed(vdr0, *reqID); err != nil {
		t.Fatal(err)
	}

	if vtx0.Status() != choices.Accepted {
		t.Fatalf("Should have executed vertex")
	}
	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Should have finished blocking")
	}
}

func TestEngineBlockedIssue(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
	}

	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{
			&avalanche.TestVertex{TestDecidable: choices.TestDecidable{
				IDV:     vtx0.IDV,
				StatusV: choices.Unknown,
			}},
		},
		HeightV: 1,
		TxsV:    []conflicts.Tx{tx0},
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(vtx1, false); err != nil {
		t.Fatal(err)
	}

	vtx1.ParentsV[0] = vtx0
	if err := te.issue(vtx0, false); err != nil {
		t.Fatal(err)
	}

	if prefs := te.Consensus.Preferences(); prefs.Len() != 1 || !prefs.Contains(vtx1.ID()) {
		t.Fatalf("Should have issued vtx1")
	}
}

func TestEngineAbandonResponse(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
	}

	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) { return nil, errUnknownVertex }

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	reqID := new(uint32)
	sender.GetF = func(vID ids.ShortID, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
	}

	if err := te.PullQuery(vdr, 0, vtx.ID()); err != nil {
		t.Fatal(err)
	}
	if err := te.GetFailed(vdr, *reqID); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Should have removed blocking event")
	}
}

func TestEngineScheduleRepoll(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)
	manager.CantEdge = false

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	requestID := new(uint32)
	sender.PushQueryF = func(_ ids.ShortSet, reqID uint32, _ ids.ID, _ []byte) {
		*requestID = reqID
	}

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}

	sender.PushQueryF = nil

	repolled := new(bool)
	sender.PullQueryF = func(_ ids.ShortSet, _ uint32, vtxID ids.ID) {
		*repolled = true
		if vtxID != vtx.ID() {
			t.Fatalf("Wrong vertex queried")
		}
	}

	if err := te.QueryFailed(vdr, *requestID); err != nil {
		t.Fatal(err)
	}

	if !*repolled {
		t.Fatalf("Should have issued a noop")
	}
}

func TestEngineRejectDoubleSpendTx(t *testing.T) {
	config := DefaultConfig()

	config.Params.BatchSize = 2

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	gTx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
	}

	utxos := []ids.ID{ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[0]},
		},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}
	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		txs := make([]conflicts.Tx, len(trs))
		for i, tr := range trs {
			switch tr {
			case tx0.Transition():
				txs[i] = tx0
			case tx1.Transition():
				txs[i] = tx1
			default:
				return nil, errMissing
			}
		}
		return &avalanche.TestVertex{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			ParentsV: []avalanche.Vertex{gVtx, mVtx},
			HeightV:  1,
			TxsV:     txs,
			BytesV:   []byte{1},
		}, nil
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	sender.CantPushQuery = false

	vm.PendingF = func() []conflicts.Transition {
		return []conflicts.Transition{tx0.Transition(), tx1.Transition()}
	}
	manager.WrapF = func(epoch uint32, tr conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch tr {
		case tx0.Transition():
			return tx0, nil
		case tx1.Transition():
			return tx1, nil
		default:
			return nil, errMissing
		}
	}
	vm.GetF = func(trID ids.ID) (conflicts.Transition, error) {
		switch trID {
		case tx0.Transition().ID():
			return tx0.Transition(), nil
		case tx1.Transition().ID():
			return tx1.Transition(), nil
		default:
			return nil, errMissing
		}
	}
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}
}

func TestEngineRejectDoubleSpendIssuedTx(t *testing.T) {
	config := DefaultConfig()

	config.Params.BatchSize = 2

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	gTx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
	}

	utxos := []ids.ID{ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[0]},
		},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true
	vm.GetF = func(trID ids.ID) (conflicts.Transition, error) {
		switch trID {
		case gTx.Transition().ID():
			return gTx.Transition(), nil
		case tx0.Transition().ID():
			return tx0.Transition(), nil
		case tx1.Transition().ID():
			return tx1.Transition(), nil
		default:
			return nil, errMissing
		}
	}
	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		txs := make([]conflicts.Tx, len(trs))
		for i, tr := range trs {
			switch tr {
			case tx0.Transition():
				txs[i] = tx0
			case tx1.Transition():
				txs[i] = tx1
			default:
				return nil, errMissing
			}
		}
		return &avalanche.TestVertex{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			ParentsV: []avalanche.Vertex{gVtx, mVtx},
			HeightV:  1,
			TxsV:     txs,
			BytesV:   []byte{1},
		}, nil
	}
	manager.WrapF = func(_ uint32, tr conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch tr {
		case tx0.Transition():
			return tx0, nil
		case tx1.Transition():
			return tx1, nil
		default:
			return nil, errMissing
		}
	}

	sender.CantPushQuery = false

	vm.PendingF = func() []conflicts.Transition { return []conflicts.Transition{tx0.Transition()} }
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}

	vm.PendingF = func() []conflicts.Transition { return []conflicts.Transition{tx1.Transition()} }
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}

	if !te.Consensus.TransitionProcessing(tx0.Transition().ID()) {
		t.Fatalf("should have issued tx0")
	}
	if te.Consensus.TransitionProcessing(tx1.Transition().ID()) {
		t.Fatalf("shouldn't have issued tx1")
	}
}

func TestEngineIssueRepoll(t *testing.T) {
	config := DefaultConfig()

	config.Params.BatchSize = 2

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	sender.PullQueryF = func(vdrs ids.ShortSet, _ uint32, vtxID ids.ID) {
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !vdrs.Equals(vdrSet) {
			t.Fatalf("Wrong query recipients")
		}
		if vtxID != gVtx.ID() && vtxID != mVtx.ID() {
			t.Fatalf("Unknown re-query")
		}
	}

	if err := te.repoll(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineReissue(t *testing.T) {
	config := DefaultConfig()
	config.Params.BatchSize = 2
	config.Params.BetaVirtuous = 5
	config.Params.BetaRogue = 5

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	gTx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
	}

	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx2 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[1]},
		},
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{gVtx, mVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
		BytesV:   []byte{40},
	}

	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{gVtx, mVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx1},
		BytesV:   []byte{41},
	}

	vtx2 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  2,
		TxsV:     []conflicts.Tx{tx2},
		BytesV:   []byte{42},
	}

	vtx3 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{gVtx, mVtx},
		HeightV:  2,
		TxsV:     []conflicts.Tx{tx2},
		BytesV:   []byte{43},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		case vtx0.ID():
			return vtx0, nil
		case vtx1.ID():
			return vtx1, nil
		case vtx2.ID():
			return vtx2, nil
		case vtx3.ID():
			return vtx3, nil
		default:
			t.Fatalf("Unknown vertex")
			panic("Should have errored")
		}
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	manager.WrapF = func(_ uint32, tr conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch tr {
		case gTx.Transition():
			return gTx, nil
		case tx0.Transition():
			return tx0, nil
		case tx1.Transition():
			return tx1, nil
		case tx2.Transition():
			return tx2, nil
		default:
			return nil, errMissing
		}
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		switch {
		case bytes.Equal(b, vtx0.Bytes()):
			return vtx0, nil
		case bytes.Equal(b, vtx1.Bytes()):
			return vtx1, nil
		case bytes.Equal(b, vtx2.Bytes()):
			return vtx2, nil
		case bytes.Equal(b, vtx3.Bytes()):
			return vtx3, nil
		default:
			t.Fatalf("Wrong bytes")
			panic("should have errored")
		}
	}

	vm.GetF = func(id ids.ID) (conflicts.Transition, error) {
		switch id {
		case gTx.Transition().ID():
			return gTx.Transition(), nil
		case tx0.Transition().ID():
			return tx0.Transition(), nil
		case tx1.Transition().ID():
			return tx1.Transition(), nil
		case tx2.Transition().ID():
			return tx2.Transition(), nil
		default:
			return nil, errMissing
		}
	}

	queryRequestID := new(uint32)
	sender.PushQueryF = func(_ ids.ShortSet, requestID uint32, _ ids.ID, _ []byte) {
		*queryRequestID = requestID
	}

	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		if len(trs) != 1 {
			t.Fatalf("wrong number of transitions")
		}
		tr := trs[0]
		if tr != tx0.Transition() {
			t.Fatalf("wrong transition used")
		}
		return vtx0, nil
	}
	vm.PendingF = func() []conflicts.Transition {
		return []conflicts.Transition{tx0.Transition()}
	}
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}

	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		if len(trs) != 1 {
			t.Fatalf("wrong number of transitions")
		}
		tr := trs[0]
		if tr != tx2.Transition() {
			t.Fatalf("wrong transition used")
		}
		return vtx2, nil
	}
	vm.PendingF = func() []conflicts.Transition {
		return []conflicts.Transition{tx2.Transition()}
	}
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}

	if err := te.Put(vdr, 0, vtx1.ID(), vtx1.Bytes()); err != nil {
		t.Fatal(err)
	}

	called := new(bool)
	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		if len(trs) != 1 {
			t.Fatalf("wrong number of transitions")
		}
		tr := trs[0]
		if tr != tx2.Transition() {
			t.Fatalf("wrong transition used")
		}
		*called = true
		return vtx3, nil
	}

	if err := te.Chits(vdr, *queryRequestID, []ids.ID{vtx1.ID()}); err != nil {
		t.Fatal(err)
	}

	if !*called {
		t.Fatalf("Should have re-issued the tx")
	}
}

func TestEngineLargeIssue(t *testing.T) {
	config := DefaultConfig()
	config.Params.BatchSize = 1
	config.Params.BetaVirtuous = 5
	config.Params.BetaRogue = 5

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	gTx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
	}

	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[1]},
		},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}
	manager.WrapF = func(_ uint32, tr conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch tr {
		case tx0.Transition():
			return tx0, nil
		case tx1.Transition():
			return tx1, nil
		default:
			return nil, errMissing
		}
	}

	vm.GetF = func(trID ids.ID) (conflicts.Transition, error) {
		switch trID {
		case tx0.Transition().ID():
			return tx0.Transition(), nil
		case tx1.Transition().ID():
			return tx1.Transition(), nil
		default:
			return nil, errMissing
		}
	}
	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	lastVtx := new(avalanche.TestVertex)
	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		txs := make([]conflicts.Tx, len(trs))
		for i, tr := range trs {
			switch tr {
			case tx0.Transition():
				txs[i] = tx0
			case tx1.Transition():
				txs[i] = tx1
			default:
				return nil, errMissing
			}
		}
		lastVtx = &avalanche.TestVertex{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			ParentsV: []avalanche.Vertex{gVtx, mVtx},
			HeightV:  1,
			TxsV:     txs,
			BytesV:   []byte{1},
		}
		return lastVtx, nil
	}

	sender.CantPushQuery = false

	vm.PendingF = func() []conflicts.Transition {
		return []conflicts.Transition{tx0.Transition(), tx1.Transition()}
	}
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}

	if len(lastVtx.TxsV) != 1 || lastVtx.TxsV[0].ID() != tx1.ID() {
		t.Fatalf("Should have issued txs differently")
	}
}

func TestEngineGetVertex(t *testing.T) {
	config := DefaultConfig()

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vdr := validators.GenerateRandomValidator(1)

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	sender.PutF = func(v ids.ShortID, _ uint32, vtxID ids.ID, vtx []byte) {
		if v != vdr.ID() {
			t.Fatalf("Wrong validator")
		}
		if mVtx.ID() != vtxID {
			t.Fatalf("Wrong vertex")
		}
	}

	if err := te.Get(vdr.ID(), 0, mVtx.ID()); err != nil {
		t.Fatal(err)
	}
}

func TestEngineInsufficientValidators(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	queried := new(bool)
	sender.PushQueryF = func(inVdrs ids.ShortSet, _ uint32, vtxID ids.ID, vtx []byte) {
		*queried = true
	}

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}

	if *queried {
		t.Fatalf("Unknown query")
	}
}

func TestEnginePushGossip(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		case vtx.ID():
			return vtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	requested := new(bool)
	sender.GetF = func(vdr ids.ShortID, _ uint32, vtxID ids.ID) {
		*requested = true
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtx.BytesV) {
			return vtx, nil
		}
		t.Fatalf("Unknown vertex bytes")
		panic("Should have errored")
	}

	sender.CantPushQuery = false
	sender.CantChits = false
	if err := te.PushQuery(vdr, 0, vtx.ID(), vtx.Bytes()); err != nil {
		t.Fatal(err)
	}

	if *requested {
		t.Fatalf("Shouldn't have requested the vertex")
	}
}

func TestEngineSingleQuery(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		case vtx.ID():
			return vtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	sender.CantPushQuery = false
	sender.CantPullQuery = false

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}
}

func TestEngineParentBlockingInsert(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	missingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	parentVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{missingVtx},
		HeightV:  2,
		BytesV:   []byte{0, 1, 2, 3},
	}

	blockingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{parentVtx},
		HeightV:  3,
		BytesV:   []byte{0, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(parentVtx, false); err != nil {
		t.Fatal(err)
	}
	if err := te.issue(blockingVtx, false); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 2 {
		t.Fatalf("Both inserts should be blocking")
	}

	sender.CantPushQuery = false

	missingVtx.StatusV = choices.Processing
	if err := te.issue(missingVtx, false); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Both inserts should not longer be blocking")
	}
}

func TestEngineBlockingChitRequest(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	missingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	parentVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{missingVtx},
		HeightV:  2,
		BytesV:   []byte{1, 1, 2, 3},
	}

	blockingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{parentVtx},
		HeightV:  3,
		BytesV:   []byte{2, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(parentVtx, false); err != nil {
		t.Fatal(err)
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == blockingVtx.ID() {
			return blockingVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, blockingVtx.Bytes()) {
			return blockingVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	if err := te.PushQuery(vdr, 0, blockingVtx.ID(), blockingVtx.Bytes()); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 3 {
		t.Fatalf("Both inserts and the query should be blocking")
	}

	sender.CantPushQuery = false
	sender.CantChits = false

	missingVtx.StatusV = choices.Processing
	if err := te.issue(missingVtx, false); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Both inserts should not longer be blocking")
	}
}

func TestEngineBlockingChitResponse(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	issuedVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	missingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{1, 1, 2, 3},
	}

	blockingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{missingVtx},
		HeightV:  2,
		BytesV:   []byte{2, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(blockingVtx, false); err != nil {
		t.Fatal(err)
	}

	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, vtx []byte) {
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if issuedVtx.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	if err := te.issue(issuedVtx, false); err != nil {
		t.Fatal(err)
	}

	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id == blockingVtx.ID() {
			return blockingVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	if err := te.Chits(vdr, *queryRequestID, []ids.ID{blockingVtx.ID()}); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 2 {
		t.Fatalf("The insert should be blocking, as well as the chit response")
	}

	sender.PushQueryF = nil
	sender.CantPushQuery = false
	sender.CantChits = false

	missingVtx.StatusV = choices.Processing
	if err := te.issue(missingVtx, false); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Both inserts should not longer be blocking")
	}
}

func TestEngineMissingTx(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}

	issuedVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{0, 1, 2, 3},
	}

	missingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   []byte{1, 1, 2, 3},
	}

	blockingVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{missingVtx},
		HeightV:  2,
		BytesV:   []byte{2, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(blockingVtx, false); err != nil {
		t.Fatal(err)
	}

	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, vtx []byte) {
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if issuedVtx.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
	}

	if err := te.issue(issuedVtx, false); err != nil {
		t.Fatal(err)
	}

	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id == blockingVtx.ID() {
			return blockingVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	if err := te.Chits(vdr, *queryRequestID, []ids.ID{blockingVtx.ID()}); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 2 {
		t.Fatalf("The insert should be blocking, as well as the chit response")
	}

	sender.PushQueryF = nil
	sender.CantPushQuery = false
	sender.CantChits = false

	missingVtx.StatusV = choices.Processing
	if err := te.issue(missingVtx, false); err != nil {
		t.Fatal(err)
	}

	if len(te.vtxBlocked) != 0 {
		t.Fatalf("Both inserts should not longer be blocking")
	}
}

func TestEngineIssueBlockingTx(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{tx0.Transition()},
			InputIDsV:     []ids.ID{utxos[1]},
		},
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0, tx1},
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}

	if prefs := te.Consensus.Preferences(); !prefs.Contains(vtx.ID()) {
		t.Fatalf("Vertex should be preferred")
	}
}

func TestEngineReissueAbortedVertex(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx}

	vtxID0 := ids.GenerateTestID()
	vtxID1 := ids.GenerateTestID()

	vtxBytes0 := []byte{0}
	vtxBytes1 := []byte{1}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     vtxID0,
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		BytesV:   vtxBytes0,
	}
	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     vtxID1,
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  2,
		BytesV:   vtxBytes1,
	}

	manager.EdgeF = func() []ids.ID {
		return []ids.ID{gVtx.ID()}
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == gVtx.ID() {
			return gVtx, nil
		}
		t.Fatalf("Unknown vertex requested")
		panic("Unknown vertex requested")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	manager.EdgeF = nil
	manager.GetF = nil

	requestID := new(uint32)
	sender.GetF = func(vID ids.ShortID, reqID uint32, vtxID ids.ID) {
		*requestID = reqID
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtxBytes1) {
			return vtx1, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}
	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == vtxID1 {
			return vtx1, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}

	if err := te.PushQuery(vdr, 0, vtxID1, vtx1.Bytes()); err != nil {
		t.Fatal(err)
	}

	sender.GetF = nil
	manager.ParseF = nil

	if err := te.GetFailed(vdr, *requestID); err != nil {
		t.Fatal(err)
	}

	requested := new(bool)
	sender.GetF = func(_ ids.ShortID, _ uint32, vtxID ids.ID) {
		if vtxID == vtxID0 {
			*requested = true
		}
	}
	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == vtxID1 {
			return vtx1, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}

	if err := te.PullQuery(vdr, 0, vtxID1); err != nil {
		t.Fatal(err)
	}

	if !*requested {
		t.Fatalf("Should have requested the missing vertex")
	}
}

func TestEngineBootstrappingIntoConsensus(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals
	config.Beacons = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	config.SampleK = int(vals.Weight())

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	txID0 := ids.GenerateTestID()
	txID1 := ids.GenerateTestID()

	txBytes0 := []byte{0}
	txBytes1 := []byte{1}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     txID0,
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
		BytesV: txBytes0,
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     txID1,
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{tx0.Transition()},
			InputIDsV:     []ids.ID{utxos[1]},
		},
		BytesV: txBytes1,
	}

	vtxID0 := ids.GenerateTestID()
	vtxID1 := ids.GenerateTestID()

	vtxBytes0 := []byte{2}
	vtxBytes1 := []byte{3}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     vtxID0,
			StatusV: choices.Processing,
		},
		HeightV: 1,
		TxsV:    []conflicts.Tx{tx0},
		BytesV:  vtxBytes0,
	}
	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     vtxID1,
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  2,
		TxsV:     []conflicts.Tx{tx1},
		BytesV:   vtxBytes1,
	}

	requested := new(bool)
	requestID := new(uint32)
	sender.GetAcceptedFrontierF = func(vdrs ids.ShortSet, reqID uint32) {
		if vdrs.Len() != 1 {
			t.Fatalf("Should have requested from the validators")
		}
		if !vdrs.Contains(vdr) {
			t.Fatalf("Should have requested from %s", vdr)
		}
		*requested = true
		*requestID = reqID
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}
	if err := te.Startup(); err != nil {
		t.Fatal(err)
	}

	sender.GetAcceptedFrontierF = nil

	if !*requested {
		t.Fatalf("Should have requested from the validators during Initialize")
	}

	acceptedFrontier := []ids.ID{vtxID0}

	*requested = false
	sender.GetAcceptedF = func(vdrs ids.ShortSet, reqID uint32, proposedAccepted []ids.ID) {
		if vdrs.Len() != 1 {
			t.Fatalf("Should have requested from the validators")
		}
		if !vdrs.Contains(vdr) {
			t.Fatalf("Should have requested from %s", vdr)
		}
		if !ids.Equals(acceptedFrontier, proposedAccepted) {
			t.Fatalf("Wrong proposedAccepted vertices.\nExpected: %s\nGot: %s", acceptedFrontier, proposedAccepted)
		}
		*requested = true
		*requestID = reqID
	}

	if err := te.AcceptedFrontier(vdr, *requestID, acceptedFrontier); err != nil {
		t.Fatal(err)
	}

	if !*requested {
		t.Fatalf("Should have requested from the validators during AcceptedFrontier")
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == vtxID0 {
			return nil, errMissing
		}
		t.Fatalf("Unknown vertex requested")
		panic("Unknown vertex requested")
	}

	sender.GetAncestorsF = func(inVdr ids.ShortID, reqID uint32, vtxID ids.ID) {
		if vdr != inVdr {
			t.Fatalf("Asking wrong validator for vertex")
		}
		if vtx0.ID() != vtxID {
			t.Fatalf("Asking for wrong vertex")
		}
		*requestID = reqID
	}

	if err := te.Accepted(vdr, *requestID, acceptedFrontier); err != nil {
		t.Fatal(err)
	}

	manager.GetF = nil
	sender.GetF = nil

	vm.ParseF = func(b []byte) (conflicts.Transition, error) {
		if bytes.Equal(b, txBytes0) {
			return tx0.Transition(), nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtxBytes0) {
			return vtx0, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}
	manager.EdgeF = func() []ids.ID {
		return []ids.ID{vtxID0}
	}
	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == vtxID0 {
			return vtx0, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}
	manager.ParseTxF = func(b []byte) (conflicts.Tx, error) {
		switch {
		case bytes.Equal(b, txBytes0):
			return tx0, nil
		case bytes.Equal(b, txBytes1):
			return tx1, nil
		default:
			return nil, errMissing
		}
	}

	if err := te.MultiPut(vdr, *requestID, [][]byte{vtxBytes0}); err != nil {
		t.Fatal(err)
	}

	vm.ParseF = nil
	manager.ParseF = nil
	manager.EdgeF = nil
	manager.GetF = nil

	if tx0.Status() != choices.Accepted {
		t.Fatalf("Should have accepted %s", txID0)
	}
	if vtx0.Status() != choices.Accepted {
		t.Fatalf("Should have accepted %s", vtxID0)
	}

	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtxBytes1) {
			return vtx1, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}
	sender.ChitsF = func(inVdr ids.ShortID, _ uint32, chits []ids.ID) {
		if inVdr != vdr {
			t.Fatalf("Sent to the wrong validator")
		}

		expected := []ids.ID{vtxID1}

		if !ids.Equals(expected, chits) {
			t.Fatalf("Returned wrong chits")
		}
	}
	sender.PushQueryF = func(vdrs ids.ShortSet, _ uint32, vtxID ids.ID, vtx []byte) {
		if vdrs.Len() != 1 {
			t.Fatalf("Should have requested from the validators")
		}
		if !vdrs.Contains(vdr) {
			t.Fatalf("Should have requested from %s", vdr)
		}

		if vtxID1 != vtxID {
			t.Fatalf("Sent wrong query ID")
		}
		if !bytes.Equal(vtxBytes1, vtx) {
			t.Fatalf("Sent wrong query bytes")
		}
	}
	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == vtxID1 {
			return vtx1, nil
		}
		t.Fatalf("Unknown bytes provided")
		panic("Unknown bytes provided")
	}

	if err := te.PushQuery(vdr, 0, vtxID1, vtxBytes1); err != nil {
		t.Fatal(err)
	}

	manager.ParseF = nil
	sender.ChitsF = nil
	sender.PushQueryF = nil
	manager.GetF = nil
}

func TestEngineUndeclaredDependencyDeadlock(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[1]},
		},
		VerifyV: errors.New(""),
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
	}
	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  2,
		TxsV:     []conflicts.Tx{tx1},
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	te.Sender = sender

	reqID := new(uint32)
	sender.PushQueryF = func(_ ids.ShortSet, requestID uint32, _ ids.ID, _ []byte) {
		*reqID = requestID
	}

	if err := te.issue(vtx0, false); err != nil {
		t.Fatal(err)
	}

	sender.PushQueryF = func(ids.ShortSet, uint32, ids.ID, []byte) {
		t.Fatalf("should have failed verification")
	}

	if err := te.issue(vtx1, false); err != nil {
		t.Fatal(err)
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		switch vtxID {
		case vtx0.ID():
			return vtx0, nil
		case vtx1.ID():
			return vtx1, nil
		}
		return nil, errors.New("Unknown vtx")
	}

	if err := te.Chits(vdr, *reqID, []ids.ID{vtx1.ID()}); err != nil {
		t.Fatal(err)
	}

	if status := vtx0.Status(); status != choices.Accepted {
		t.Fatalf("should have accepted the vertex due to transitive voting")
	}
}

func TestEnginePartiallyValidVertex(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[1]},
		},
		VerifyV: errors.New(""),
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0, tx1},
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	expectedVtxID := ids.GenerateTestID()
	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		txs := make([]conflicts.Tx, len(trs))
		for i, tr := range trs {
			switch tr {
			case tx0.Transition():
				txs[i] = tx0
			case tx1.Transition():
				txs[i] = tx1
			default:
				return nil, errMissing
			}
		}
		return &avalanche.TestVertex{
			TestDecidable: choices.TestDecidable{
				IDV:     expectedVtxID,
				StatusV: choices.Processing,
			},
			ParentsV: vts,
			HeightV:  1,
			TxsV:     txs,
			BytesV:   []byte{1},
		}, nil
	}
	manager.WrapF = func(_ uint32, tr conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch tr {
		case tx0.Transition():
			return tx0, nil
		case tx1.Transition():
			return tx1, nil
		default:
			return nil, errMissing
		}
	}

	sender := &common.SenderTest{}
	sender.T = t
	te.Sender = sender

	sender.PushQueryF = func(_ ids.ShortSet, _ uint32, vtxID ids.ID, _ []byte) {
		if expectedVtxID != vtxID {
			t.Fatalf("wrong vertex queried")
		}
	}

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}
}

func TestEngineGossip(t *testing.T) {
	config := DefaultConfig()

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID()} }
	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if vtxID == gVtx.ID() {
			return gVtx, nil
		}
		t.Fatal(errUnknownVertex)
		return nil, errUnknownVertex
	}

	called := new(bool)
	sender.GossipF = func(vtxID ids.ID, vtxBytes []byte) {
		*called = true
		if vtxID != gVtx.ID() {
			t.Fatal(errUnknownVertex)
		}
		if !bytes.Equal(vtxBytes, gVtx.Bytes()) {
			t.Fatal(errUnknownVertex)
		}
	}

	if err := te.Gossip(); err != nil {
		t.Fatal(err)
	}

	if !*called {
		t.Fatalf("Should have gossiped the vertex")
	}
}

func TestEngineInvalidVertexIgnoredFromUnexpectedPeer(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	secondVdr := ids.GenerateTestShortID()

	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}
	if err := vals.AddWeight(secondVdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		BytesV: []byte{0},
	}

	vts := []avalanche.Vertex{gVtx}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[1]},
		},
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
		BytesV:   []byte{1},
	}
	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  2,
		TxsV:     []conflicts.Tx{tx1},
		BytesV:   []byte{2},
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	parsed := new(bool)
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtx1.Bytes()) {
			*parsed = true
			return vtx1, nil
		}
		return nil, errUnknownVertex
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if !*parsed {
			return nil, errUnknownVertex
		}

		if vtxID == vtx1.ID() {
			return vtx1, nil
		}
		return nil, errUnknownVertex
	}

	reqID := new(uint32)
	sender.GetF = func(reqVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
		if reqVdr != vdr {
			t.Fatalf("Wrong validator requested")
		}
		if vtxID != vtx0.ID() {
			t.Fatalf("Wrong vertex requested")
		}
	}

	if err := te.PushQuery(vdr, 0, vtx1.ID(), vtx1.Bytes()); err != nil {
		t.Fatal(err)
	}

	if err := te.Put(secondVdr, *reqID, vtx0.ID(), []byte{3}); err != nil {
		t.Fatal(err)
	}

	*parsed = false
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtx0.Bytes()) {
			*parsed = true
			return vtx0, nil
		}
		return nil, errUnknownVertex
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if !*parsed {
			return nil, errUnknownVertex
		}

		if vtxID == vtx0.ID() {
			return vtx0, nil
		}
		return nil, errUnknownVertex
	}
	sender.CantPushQuery = false
	sender.CantChits = false

	vtx0.StatusV = choices.Processing

	if err := te.Put(vdr, *reqID, vtx0.ID(), vtx0.Bytes()); err != nil {
		t.Fatal(err)
	}

	prefs := te.Consensus.Preferences()
	if !prefs.Contains(vtx1.ID()) {
		t.Fatalf("Shouldn't have abandoned the pending vertex")
	}
}

func TestEnginePushQueryRequestIDConflict(t *testing.T) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	gVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		BytesV: []byte{0},
	}

	vts := []avalanche.Vertex{gVtx}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	tx1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[1]},
		},
	}

	vtx0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
		BytesV:   []byte{1},
	}

	vtx1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  2,
		TxsV:     []conflicts.Tx{tx1},
		BytesV:   []byte{2},
	}

	randomVtxID := ids.GenerateTestID()

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	parsed := new(bool)
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtx1.Bytes()) {
			*parsed = true
			return vtx1, nil
		}
		return nil, errUnknownVertex
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if !*parsed {
			return nil, errUnknownVertex
		}

		if vtxID == vtx1.ID() {
			return vtx1, nil
		}
		return nil, errUnknownVertex
	}

	reqID := new(uint32)
	sender.GetF = func(reqVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
		if reqVdr != vdr {
			t.Fatalf("Wrong validator requested")
		}
		if vtxID != vtx0.ID() {
			t.Fatalf("Wrong vertex requested")
		}
	}

	if err := te.PushQuery(vdr, 0, vtx1.ID(), vtx1.Bytes()); err != nil {
		t.Fatal(err)
	}

	sender.GetF = nil
	sender.CantGet = false

	if err := te.PushQuery(vdr, *reqID, randomVtxID, []byte{3}); err != nil {
		t.Fatal(err)
	}

	*parsed = false
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtx0.Bytes()) {
			*parsed = true
			return vtx0, nil
		}
		return nil, errUnknownVertex
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if !*parsed {
			return nil, errUnknownVertex
		}

		if vtxID == vtx0.ID() {
			return vtx0, nil
		}
		return nil, errUnknownVertex
	}
	sender.CantPushQuery = false
	sender.CantChits = false

	vtx0.StatusV = choices.Processing

	if err := te.Put(vdr, *reqID, vtx0.ID(), vtx0.Bytes()); err != nil {
		t.Fatal(err)
	}

	prefs := te.Consensus.Preferences()
	if !prefs.Contains(vtx1.ID()) {
		t.Fatalf("Shouldn't have abandoned the pending vertex")
	}
}

func TestEngineAggressivePolling(t *testing.T) {
	config := DefaultConfig()

	config.Params.ConcurrentRepolls = 3
	config.Params.BetaRogue = 3

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	gVtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		BytesV: []byte{0},
	}

	vts := []avalanche.Vertex{gVtx}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx},
		BytesV:   []byte{1},
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	parsed := new(bool)
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if bytes.Equal(b, vtx.Bytes()) {
			*parsed = true
			return vtx, nil
		}
		return nil, errUnknownVertex
	}

	manager.GetF = func(vtxID ids.ID) (avalanche.Vertex, error) {
		if !*parsed {
			return nil, errUnknownVertex
		}

		if vtxID == vtx.ID() {
			return vtx, nil
		}
		return nil, errUnknownVertex
	}

	numPushQueries := new(int)
	sender.PushQueryF = func(ids.ShortSet, uint32, ids.ID, []byte) { *numPushQueries++ }

	numPullQueries := new(int)
	sender.PullQueryF = func(ids.ShortSet, uint32, ids.ID) { *numPullQueries++ }

	vm.CantPending = false

	if err := te.Put(vdr, 0, vtx.ID(), vtx.Bytes()); err != nil {
		t.Fatal(err)
	}

	if *numPushQueries != 1 {
		t.Fatalf("should have issued one push query")
	}
	if *numPullQueries != 2 {
		t.Fatalf("should have issued two pull queries")
	}
}

func TestEngineDuplicatedIssuance(t *testing.T) {
	config := DefaultConfig()
	config.Params.BatchSize = 1
	config.Params.BetaVirtuous = 5
	config.Params.BetaRogue = 5

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr, 1); err != nil {
		t.Fatal(err)
	}

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	gTx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
	}

	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	tx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{gTx.Transition()},
			InputIDsV:     []ids.ID{utxos[0]},
		},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	lastVtx := new(avalanche.TestVertex)
	manager.BuildF = func(_ uint32, _ []ids.ID, trs []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		txs := make([]conflicts.Tx, len(trs))
		for i, tr := range trs {
			if tr == tx.Transition() {
				txs[i] = tx
			} else {
				return nil, errMissing
			}
		}
		lastVtx = &avalanche.TestVertex{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			ParentsV: []avalanche.Vertex{gVtx, mVtx},
			HeightV:  1,
			TxsV:     txs,
			BytesV:   []byte{1},
		}
		return lastVtx, nil
	}
	manager.WrapF = func(_ uint32, tr conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch tr {
		case tx.Transition():
			return tx, nil
		default:
			return nil, errMissing
		}
	}

	sender.CantPushQuery = false

	vm.PendingF = func() []conflicts.Transition { return []conflicts.Transition{tx.Transition()} }
	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}

	if len(lastVtx.TxsV) != 1 || lastVtx.TxsV[0].ID() != tx.ID() {
		t.Fatalf("Should have issued txs differently")
	}

	manager.BuildF = func(uint32, []ids.ID, []conflicts.Transition, []ids.ID) (avalanche.Vertex, error) {
		t.Fatalf("shouldn't have attempted to issue a duplicated tx")
		return nil, nil
	}

	if err := te.Notify(common.PendingTxs); err != nil {
		t.Fatal(err)
	}
}

// Assert that an invalid vertex is not built when an invalid transaction is
// issued.
func TestEngineNoInvalidVertices(t *testing.T) {
	config := DefaultConfig()

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
		VerifyV: errors.New("invalid tx"),
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx},
		BytesV:   []byte{1, 1, 2, 3},
	}
	manager.BuildF = func(uint32, []ids.ID, []conflicts.Transition, []ids.ID) (avalanche.Vertex, error) {
		t.Fatalf("invalid vertex built")
		panic("invalid vertex built")
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		case vtx.ID():
			return vtx, nil
		default:
			t.Fatalf("Unknown vertex")
			panic("Should have errored")
		}
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}
}

func TestEngineDoubleChit(t *testing.T) {
	config := DefaultConfig()

	config.Params.Alpha = 2
	config.Params.K = 2

	vals := validators.NewSet()
	config.Validators = vals

	vdr0 := ids.GenerateTestShortID()
	vdr1 := ids.GenerateTestShortID()

	if err := vals.AddWeight(vdr0, 1); err != nil {
		t.Fatal(err)
	}
	if err := vals.AddWeight(vdr1, 1); err != nil {
		t.Fatal(err)
	}

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	manager := vertex.NewTestManager(t)
	config.Manager = manager

	manager.Default(true)

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{utxos[0]},
		},
	}

	vtx := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx},
		BytesV:   []byte{1, 1, 2, 3},
	}

	manager.EdgeF = func() []ids.ID { return []ids.ID{vts[0].ID(), vts[1].ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	reqID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, _ []byte) {
		*reqID = requestID
		if inVdrs.Len() != 2 {
			t.Fatalf("Wrong number of validators")
		}
		if vtxID != vtx.ID() {
			t.Fatalf("Wrong vertex requested")
		}
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id == vtx.ID() {
			return vtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}

	if err := te.issue(vtx, false); err != nil {
		t.Fatal(err)
	}

	votes := []ids.ID{vtx.ID()}

	if status := tx.Status(); status != choices.Processing {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Processing)
	}
	if err := te.Chits(vdr0, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	if status := tx.Status(); status != choices.Processing {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Processing)
	}
	if err := te.Chits(vdr0, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	if status := tx.Status(); status != choices.Processing {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Processing)
	}
	if err := te.Chits(vdr1, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	if status := tx.Status(); status != choices.Accepted {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Accepted)
	}
}

// Regression test to ensure a Chit message containing a vertex whose ancestor
// fails verification does not cause a voter to register a dependency that has
// already been abandoned.
func TestEngineInvalidChit(t *testing.T) {
	config := DefaultConfig()
	config.Params.BetaVirtuous = 2
	config.Params.BetaRogue = 2
	config.Params.Alpha = 2
	config.Params.K = 2
	vals := validators.NewSet()
	config.Validators = vals
	vdr0 := ids.GenerateTestShortID()
	vdr1 := ids.GenerateTestShortID()
	if err := vals.AddWeight(vdr0, 1); err != nil {
		t.Fatal(err)
	}
	if err := vals.AddWeight(vdr1, 1); err != nil {
		t.Fatal(err)
	}
	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender
	sender.Default(true)
	sender.CantGetAcceptedFrontier = false
	manager := vertex.NewTestManager(t)
	config.Manager = manager
	manager.Default(true)
	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	mVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	manager.EdgeF = func() []ids.ID { return []ids.ID{gVtx.ID(), mVtx.ID()} }
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case gVtx.ID():
			return gVtx, nil
		case mVtx.ID():
			return mVtx, nil
		}
		t.Fatalf("Unknown vertex")
		panic("Should have errored")
	}
	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}
	vts := []avalanche.Vertex{gVtx, mVtx}
	utxos := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}
	trA := &conflicts.TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{utxos[0]},
	}
	trB := &conflicts.TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{utxos[1]},
	}
	trC := &conflicts.TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{utxos[2]},
	}
	trD := &conflicts.TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Processing,
		InputIDsV: []ids.ID{utxos[3]},
	}
	txA0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trA,
		EpochV:      0,
	}
	txB0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trB,
		EpochV:      0,
	}
	txC0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trC,
		EpochV:      0,
	}
	txC1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trC,
		EpochV:      1,
	}
	txD0 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		TransitionV: trD,
		EpochV:      0,
	}
	vtxA0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		EpochV:   0,
		TxsV:     []conflicts.Tx{txA0},
		BytesV:   []byte{1},
	}
	vtxB0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentsV: vts,
		HeightV:  1,
		EpochV:   0,
		TxsV:     []conflicts.Tx{txB0},
		BytesV:   []byte{2},
	}
	vtxC0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  2,
		EpochV:   0,
		TxsV:     []conflicts.Tx{txC0},
		BytesV:   []byte{3},
	}
	vtxC1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  2,
		EpochV:   1,
		TxsV:     []conflicts.Tx{txC1},
		BytesV:   []byte{4},
	}
	vtxD0 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []avalanche.Vertex{vtxC1, vtxB0},
		HeightV:  3,
		EpochV:   1,
		TxsV:     []conflicts.Tx{txD0},
		BytesV:   []byte{5},
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case vtxA0.ID():
			return vtxA0, nil
		case vtxC0.ID():
			return vtxC0, nil
		case vtxC1.ID():
			return vtxC1, nil
		case vtxD0.ID():
			return vtxD0, nil
		default:
			return nil, errMissing
		}
	}
	reqID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID, _ []byte) {
		*reqID = requestID
		if inVdrs.Len() != 2 {
			t.Fatalf("Wrong number of validators")
		}
		if vtxID != vtxA0.ID() {
			t.Fatalf("Wrong vertex requested")
		}
	}
	if err := te.issue(vtxA0, false); err != nil {
		t.Fatal(err)
	}
	votes := []ids.ID{vtxA0.ID()}
	if err := te.Chits(vdr0, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	sender.PullQueryF = func(inVdrs ids.ShortSet, requestID uint32, vtxID ids.ID) {
		*reqID = requestID
		if inVdrs.Len() != 2 {
			t.Fatalf("Wrong number of validators")
		}
	}
	if err := te.Chits(vdr1, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	if status := txA0.Status(); status != choices.Processing {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Processing)
	}
	manager.WrapF = func(epoch uint32, tran conflicts.Transition, _ []ids.ID) (conflicts.Tx, error) {
		switch epoch {
		case 0:
			switch tran {
			case trA:
				return txA0, nil
			case trB:
				return txB0, nil
			case trC:
				return txC0, nil
			case trD:
				return txD0, nil
			}
		case 1:
			if tran == trC {
				return txC1, nil
			}
		}
		t.Fatalf("wrong epoch")
		panic("Should have errored")
	}
	manager.BuildF = func(epoch uint32, _ []ids.ID, _ []conflicts.Transition, _ []ids.ID) (avalanche.Vertex, error) {
		return vtxB0, nil
	}
	sender.CantPushQuery = false
	sender.PushQueryF = nil
	if err := te.Chits(vdr0, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	if status := txA0.Status(); status != choices.Processing {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Processing)
	}
	askedVdrID := new(ids.ShortID)
	sender.GetF = func(vdrID ids.ShortID, requestID uint32, _ ids.ID) {
		*reqID = requestID
		*askedVdrID = vdrID
	}
	votes = []ids.ID{vtxA0.ID(), vtxD0.ID()}
	if err := te.Chits(vdr1, *reqID, votes); err != nil {
		t.Fatal(err)
	}
	if err := te.GetFailed(*askedVdrID, *reqID); err != nil {
		t.Fatal(err)
	}
	if status := txA0.Status(); status != choices.Accepted {
		t.Fatalf("Wrong tx status: %s ; expected: %s", status, choices.Accepted)
	}
}

// Test that the engine issues a transaction only if all the transition's
// dependencies are accepted in an earlier epoch or processing in the
// transaction's epoch
func TestEngineTransitionDependencyFulfilled(t *testing.T) {
	config := DefaultConfig()
	vals := validators.NewSet()
	config.Validators = vals
	vdr := ids.GenerateTestShortID()
	err := vals.AddWeight(vdr, 1)
	assert.NoError(t, err)
	sender := &common.SenderTest{}
	sender.T = t
	sender.CantGet = false
	config.Sender = sender
	manager := vertex.NewTestManager(t)
	config.Manager = manager
	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm
	vm.Default(true)
	manager.Default(true)
	manager.CantEdge = false
	vm.CantBootstrapping = false
	vm.CantBootstrapped = false
	vm.CantParse = false
	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	tx0 := &conflicts.TestTx{
		BytesV: []byte{0},
		EpochV: te.Ctx.Epoch(),
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Unknown,
			InputIDsV: []ids.ID{ids.GenerateTestID()},
		},
	}
	tx1 := &conflicts.TestTx{ // Depends on tx0's transition
		BytesV: []byte{1},
		EpochV: tx0.Epoch() + 1, // 1 epoch after tx0
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Unknown,
			InputIDsV:     []ids.ID{ids.GenerateTestID()},
			DependenciesV: []conflicts.Transition{tx0.Transition()},
		},
	}
	tx2 := &conflicts.TestTx{ // Depends on tx0 and tx1
		BytesV: []byte{2},
		EpochV: tx1.Epoch(), // Same epoch as tx1
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Unknown,
			InputIDsV:     []ids.ID{ids.GenerateTestID()},
			DependenciesV: []conflicts.Transition{tx0.Transition(), tx1.Transition()},
		},
	}

	vtx0 := &avalanche.TestVertex{ // contains tx0
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   tx0.Epoch(),
		ParentsV: []avalanche.Vertex{gVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{tx0},
		BytesV:   []byte{3},
	}
	vtx1 := &avalanche.TestVertex{ // contains tx1 and tx2
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   tx1.Epoch(),
		ParentsV: []avalanche.Vertex{vtx0},
		HeightV:  vtx0.HeightV + 1,
		TxsV:     []conflicts.Tx{tx1, tx2},
		BytesV:   []byte{4},
	}

	// Tell the engine about vtx1
	// Expect it to ask for vtx0
	// Expect it to parse vtx1, tx1, tx2
	sentGet := new(bool)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		assert.False(t, *sentGet, "Sent get multiple times")
		*sentGet = true
		assert.Equal(t, vdr, inVdr)
		assert.Equal(t, vtx0.ID(), vtxID)
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, vtx1.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return vtx1, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case vtx0.ID():
			return nil, errors.New("not found")
		case vtx1.ID():
			vtx1.StatusV = choices.Processing
			return vtx1, nil
		default:
			return nil, errors.New("asked for wrong vertex")
		}
	}
	manager.ParseTxF = func(b []byte) (conflicts.Tx, error) {
		switch {
		case bytes.Equal(b, tx1.Bytes()):
			tx1.StatusV = choices.Processing
			tx1.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
			return tx0, nil
		case bytes.Equal(b, tx2.Bytes()):
			tx2.StatusV = choices.Processing
			tx2.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
			return tx1, nil
		default:
			return nil, errMissing
		}
	}
	err = te.PushQuery(vdr, 0, vtx1.ID(), vtx1.Bytes())
	assert.NoError(t, err)
	assert.True(t, *sentGet, "should have requested vtx0")

	// tx1 and tx2 are both waiting on tx0's transition
	assert.NotNil(t, te.missingTransitions[tx1.Epoch()])
	assert.Contains(t, te.missingTransitions[tx1.Epoch()], tx0.Transition().ID())
	assert.NotNil(t, te.trBlocked)
	assert.Len(t, te.trBlocked, 1)
	assert.Len(t, te.trBlocked[tx1.Epoch()], 1) // tx0
	assert.NotNil(t, te.trBlocked[tx1.Epoch()][tx0.Transition().ID()])
	assert.Len(t, te.trBlocked[tx1.Epoch()][tx0.Transition().ID()], 1) // vtx1

	// Fulfill the dependency for vtx0
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		if !bytes.Equal(b, vtx0.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		vtx0.StatusV = choices.Processing
		tx0.StatusV = choices.Processing
		tx0.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
		return vtx0, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case vtx0.ID():
			vtx0.StatusV = choices.Processing
			return vtx0, nil
		case vtx1.ID():
			return vtx1, nil
		default:
			return nil, errors.New("asked for wrong vertex")
		}
	}
	pushQueryReqID := new(uint32)
	pushQueryCalled := new(bool)
	sender.PushQueryF = func(_ ids.ShortSet, reqID uint32, _ ids.ID, _ []byte) {
		*pushQueryReqID = reqID
		*pushQueryCalled = true
	}
	err = te.PushQuery(vdr, 1, vtx0.ID(), vtx0.Bytes())
	assert.NoError(t, err)
	// vtx1, tx1, tx2 should not be issued yet; they can't be until
	// tx0's transition is accepted in epoch <= tx1.Epoch()
	assert.NotNil(t, te.missingTransitions[tx1.Epoch()])
	assert.NotNil(t, te.missingTransitions[tx1.Epoch()][tx0.Transition().ID()])
	assert.Len(t, te.trBlocked, 1)              // tx0.Epoch()
	assert.Len(t, te.trBlocked[tx1.Epoch()], 1) // tx0.Transition()
	assert.NotNil(t, te.trBlocked[tx1.Epoch()][tx0.Transition().ID()])
	assert.Len(t, te.trBlocked[tx1.Epoch()][tx0.Transition().ID()], 1) // vtx1

	// vtx0 should be issued
	assert.True(t, te.Consensus.VertexIssued(vtx0))
	// tx0 should be issued
	assert.True(t, te.Consensus.TxIssued(tx0))
	assert.True(t, *pushQueryCalled)
	assert.False(t, te.Consensus.VertexIssued(vtx1))
	assert.False(t, te.Consensus.TxIssued(tx1))
	assert.False(t, te.Consensus.TxIssued(tx2))

	// Send chits for vtx0, which should cause it to be accepted,
	// which will cause vtx1 to be ready to issue. vtx1's epoch is ahead
	// of the current epoch, so the engine will re-wrap tx1 and tx2's transitions
	// into new txs.
	manager.CantWrap = false
	manager.WrapF = func(epoch uint32, tr conflicts.Transition, restrictions []ids.ID) (conflicts.Tx, error) {
		if trID := tr.ID(); trID != tx1.Transition().ID() && trID != tx2.Transition().ID() {
			return nil, errors.New("expected to wrap tx1 or tx2's transition")
		}
		return &conflicts.TestTx{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			TransitionV:   tr,
			RestrictionsV: restrictions,
			EpochV:        epoch,
			BytesV:        utils.RandomBytes(32),
		}, nil
	}
	manager.CantBuild = false
	manager.BuildF = func(epoch uint32, parentIDs []ids.ID, trs []conflicts.Transition, restrictions []ids.ID) (avalanche.Vertex, error) {
		switch {
		case len(trs) != 1:
			return nil, fmt.Errorf("expected 1 transition but got %d", len(trs))
		case trs[0].ID() != tx1.Transition().ID() && trs[0].ID() != tx2.Transition().ID():
			return nil, errors.New("expected transition to be tx1 or tx2's")
		}
		return &avalanche.TestVertex{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.GenerateTestID(),
				StatusV: choices.Processing,
			},
			ParentsV: []avalanche.Vertex{vtx0}, // Ignore [parentIDs] and make vtx0 the parent
			EpochV:   epoch,
			HeightV:  vtx0.HeightV,
			TxsV: []conflicts.Tx{
				&conflicts.TestTx{
					TestDecidable: choices.TestDecidable{
						IDV:     ids.GenerateTestID(),
						StatusV: choices.Processing,
					},
					TransitionV:   trs[0],
					EpochV:        epoch,
					BytesV:        utils.RandomBytes(32),
					RestrictionsV: restrictions,
				},
			},
			BytesV: utils.RandomBytes(32),
		}, nil
	}

	// Send chits for vtx0, which should cause it to be accepted
	err = te.Chits(vdr, *pushQueryReqID, []ids.ID{vtx0.ID()})
	assert.NoError(t, err)

	// vtx1, tx1, tx2 should be issued now
	assert.True(t, te.Consensus.TransitionProcessing(tx1.Transition().ID()))
	assert.True(t, te.Consensus.TransitionProcessing(tx2.Transition().ID()))
}

// Test that the engine abandons a vertex containing a transition
// whose dependency is not expected to be issued
func TestEngineTransitionDependencyAbandoned(t *testing.T) {
	// Setup
	config := DefaultConfig()
	vals := validators.NewSet()
	config.Validators = vals
	vdr := ids.GenerateTestShortID()
	err := vals.AddWeight(vdr, 1)
	assert.NoError(t, err)
	sender := &common.SenderTest{}
	sender.T = t
	sender.CantGet = false
	config.Sender = sender
	manager := vertex.NewTestManager(t)
	config.Manager = manager
	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm
	vm.Default(true)
	manager.Default(true)
	manager.CantEdge = false
	vm.CantBootstrapping = false
	vm.CantBootstrapped = false
	vm.CantParse = false
	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}

	// Scenario: gVtx is accepted
	// vtxA contains txA
	// vtxB contains txB0, whose transition depends on txA's transition
	// vtxB contains txB1, whose transition depends on a non-existent transition
	// vtxC contains txC, whose transition depends on txB1's transition
	// Engine gets a pushquery with vtxC, then asks for and receives vtxB
	// Engine asks for then receives vtxA
	// Engine realizes it won't get the non-existent transition,
	// abandons vtxB and vtxC
	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	currentEpoch := te.Ctx.Epoch()
	txA := &conflicts.TestTx{
		BytesV: []byte{0},
		EpochV: currentEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Unknown,
			InputIDsV: []ids.ID{ids.GenerateTestID()},
		},
	}
	txB0 := &conflicts.TestTx{ // Depends on tx0's transition
		BytesV: utils.RandomBytes(32),
		EpochV: currentEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Unknown,
			InputIDsV:     []ids.ID{ids.GenerateTestID()},
			DependenciesV: []conflicts.Transition{txA.Transition()},
		},
	}
	nonexistentTr := &conflicts.TestTransition{
		IDV:       ids.GenerateTestID(),
		StatusV:   choices.Unknown,
		InputIDsV: []ids.ID{ids.GenerateTestID()},
	}
	txB1 := &conflicts.TestTx{
		BytesV: utils.RandomBytes(32),
		EpochV: currentEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Unknown,
			InputIDsV:     []ids.ID{ids.GenerateTestID()},
			DependenciesV: []conflicts.Transition{txA.Transition(), nonexistentTr},
		},
	}
	txC := &conflicts.TestTx{
		BytesV: utils.RandomBytes(32),
		EpochV: currentEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           ids.GenerateTestID(),
			StatusV:       choices.Unknown,
			InputIDsV:     []ids.ID{ids.GenerateTestID()},
			DependenciesV: []conflicts.Transition{txB1.Transition()},
		},
	}
	vtxA := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   currentEpoch,
		ParentsV: []avalanche.Vertex{gVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{txA},
		BytesV:   utils.RandomBytes(32),
	}
	vtxB := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   currentEpoch,
		ParentsV: []avalanche.Vertex{vtxA},
		HeightV:  vtxA.HeightV + 1,
		TxsV:     []conflicts.Tx{txB0, txB1},
		BytesV:   utils.RandomBytes(32),
	}
	vtxC := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   currentEpoch,
		ParentsV: []avalanche.Vertex{vtxB},
		HeightV:  vtxB.HeightV + 1,
		TxsV:     []conflicts.Tx{txC},
		BytesV:   utils.RandomBytes(32),
	}

	// Tell the engine about vtxC
	// Expect it to ask for vtxB
	// Expect it to parse vtxC, txC
	sentGet := new(bool)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		assert.False(t, *sentGet, "Sent get multiple times")
		*sentGet = true
		assert.Equal(t, vdr, inVdr)
		assert.Equal(t, vtxB.ID(), vtxID)
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		assert.Equal(t, b, vtxC.Bytes())
		vtxC.StatusV = choices.Processing
		txC.StatusV = choices.Processing
		txC.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
		return vtxC, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		assert.Contains(t, []ids.ID{vtxC.ID(), vtxB.ID()}, id)
		if id == vtxC.ID() {
			return vtxC, nil
		}
		return nil, errors.New("not found")
	}
	err = te.PushQuery(vdr, 0, vtxC.ID(), vtxC.Bytes())
	assert.NoError(t, err)
	assert.True(t, *sentGet, "should have requested vtxC")

	// txC is waiting on txB1's transition
	assert.NotNil(t, te.missingTransitions[currentEpoch])
	assert.Contains(t, te.missingTransitions[currentEpoch], txB1.Transition().ID())
	assert.NotNil(t, te.trBlocked)
	assert.Len(t, te.trBlocked, 1)
	assert.Len(t, te.trBlocked[currentEpoch], 1)                         // txB1
	assert.Len(t, te.trBlocked[currentEpoch][txB1.Transition().ID()], 1) // vtxC

	// Give the engine vtxB.
	// Expect it to parse VtxB, txB0, txB1
	// Expect it to ask for vtxB
	*sentGet = false
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, vtxID ids.ID) {
		assert.False(t, *sentGet, "Sent get multiple times")
		*sentGet = true
		assert.Equal(t, vdr, inVdr)
		assert.Equal(t, vtxA.ID(), vtxID)
	}
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		assert.Equal(t, b, vtxB.Bytes())
		vtxB.StatusV = choices.Processing
		txB0.StatusV = choices.Processing
		txB0.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
		txB1.StatusV = choices.Processing
		txB1.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
		return vtxB, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case vtxC.ID():
			return vtxC, nil
		case vtxB.ID():
			return vtxB, nil
		case vtxA.ID():
			return nil, errors.New("asked for wrong vertex")
		}
		assert.FailNow(t, "tried to get wrong vertex")
		return nil, errors.New("not found")
	}
	pushQueryReqID := new(uint32)
	pushQueryCalled := new(bool)
	sender.PushQueryF = func(_ ids.ShortSet, reqID uint32, _ ids.ID, _ []byte) {
		*pushQueryReqID = reqID
		*pushQueryCalled = true
	}
	err = te.Put(vdr, 1, vtxB.ID(), vtxB.Bytes())
	assert.NoError(t, err)
	assert.Len(t, te.missingTransitions[currentEpoch], 3)
	assert.NotNil(t, te.missingTransitions[currentEpoch][txB1.Transition().ID()])
	assert.NotNil(t, te.missingTransitions[currentEpoch][txA.Transition().ID()])
	assert.NotNil(t, te.missingTransitions[currentEpoch][nonexistentTr.ID()])
	assert.Len(t, te.trBlocked, 1)
	assert.Len(t, te.trBlocked[currentEpoch], 3)                         // txB1, txA, nonExistentTr
	assert.Len(t, te.trBlocked[currentEpoch][txA.Transition().ID()], 1)  // vtxB
	assert.Len(t, te.trBlocked[currentEpoch][txB1.Transition().ID()], 1) // vtxC
	assert.Len(t, te.trBlocked[currentEpoch][nonexistentTr.ID()], 1)     // vtxB

	// Give the engine vtxA. Expect it to parse vtxA and txA.
	// Expect it to send a PushQuery for vtxA
	sender.CantGet = false
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		assert.Equal(t, b, vtxA.Bytes())
		vtxA.StatusV = choices.Processing
		txA.StatusV = choices.Processing
		txA.TransitionV.(*conflicts.TestTransition).StatusV = choices.Processing
		return vtxA, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		switch id {
		case vtxC.ID():
			return vtxC, nil
		case vtxB.ID():
			return vtxB, nil
		case vtxA.ID():
			return vtxA, nil
		}
		assert.FailNow(t, "tried to get wrong vertex")
		return nil, errors.New("not found")
	}
	err = te.Put(vdr, 1, vtxA.ID(), vtxA.Bytes())
	assert.NoError(t, err)
	assert.NotNil(t, te.missingTransitions[currentEpoch])
	assert.NotNil(t, te.missingTransitions[currentEpoch][txB1.Transition().ID()])
	assert.NotNil(t, te.missingTransitions[currentEpoch][txA.Transition().ID()])
	assert.Len(t, te.trBlocked, 1)
	assert.Len(t, te.trBlocked[currentEpoch], 2)                         // txB1, nonExistentTr
	assert.Len(t, te.trBlocked[currentEpoch][txB1.Transition().ID()], 1) // vtxC
	assert.Len(t, te.trBlocked[currentEpoch][nonexistentTr.ID()], 1)     // vtxB
	assert.True(t, te.Consensus.VertexIssued(vtxA))
	assert.True(t, te.Consensus.TransitionProcessing(txA.Transition().ID()))
	assert.True(t, *pushQueryCalled)

	// At this point, vtxA should be issued. VtxB can't be issued because it's missing
	// its non-existing transition dependency. We only abandon transition dependencies
	// once consensus is finalized, so let's finalize consensus by sending chits for vtxA.
	err = te.Chits(vdr, *pushQueryReqID, []ids.ID{vtxA.ID()})
	assert.NoError(t, err)

	// Now we should have abandoned hope of receiving nonExistentTr, and as such
	// we have abandoned vtxB and vtxC
	assert.True(t, te.Consensus.Finalized())
	assert.Len(t, te.missingTransitions, 0)
	assert.Len(t, te.trBlocked, 0)
}

func TestEngineAbandonDependencyFulfilledInFutureEpoch(t *testing.T) {
	// Setup
	config := DefaultConfig()
	vals := validators.NewSet()
	config.Validators = vals
	vdr := ids.GenerateTestShortID()
	err := vals.AddWeight(vdr, 1)
	assert.NoError(t, err)
	sender := &common.SenderTest{}
	sender.T = t
	sender.CantGet = false
	config.Sender = sender
	manager := vertex.NewTestManager(t)
	config.Manager = manager
	vm := &vertex.TestVM{}
	vm.T = t
	config.VM = vm
	vm.Default(true)
	manager.Default(true)
	manager.CantEdge = false
	vm.CantBootstrapping = false
	vm.CantBootstrapped = false
	vm.CantParse = false
	te := &Transitive{}
	if err := te.Initialize(config); err != nil {
		t.Fatal(err)
	}
	te.Ctx.Clock.Set(te.Ctx.EpochFirstTransition.Add(1 * te.Ctx.EpochDuration))

	// Scenario:
	// gVtx is accepted and the only vtx in the frontier
	// trC depends on trB and trA
	// trA is in vtxA1 and vtxA2 in epochs 1 and 2 respectively
	// both are issued to consensus
	// trC is added to vtxC and is pending issuance on trB getting
	// added to consensus.
	// engine receives two Chits messages for vtxA2 (requires two because
	// it is not virtuous once it's issued in two epochs).
	// Thus trA should be accepted into epoch 2, so the issuer for vtxC
	// should be abandoned since its dependency will not be fulfilled in
	// its epoch.
	currentEpoch := te.Ctx.Epoch()
	priorEpoch := currentEpoch - 1
	gVtx := &avalanche.TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	trA := &conflicts.TestTransition{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Unknown,
	}
	trB := &conflicts.TestTransition{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Unknown,
	}
	trC := &conflicts.TestTransition{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Unknown,
		DependenciesV: []conflicts.Transition{
			trA,
			trB,
		},
	}
	txA1 := &conflicts.TestTx{
		BytesV: utils.RandomBytes(32),
		EpochV: priorEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: trA,
	}
	txA2 := &conflicts.TestTx{
		BytesV: utils.RandomBytes(32),
		EpochV: currentEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: trA,
	}
	txB := &conflicts.TestTx{
		BytesV: utils.RandomBytes(32),
		EpochV: priorEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: trB,
	}
	txC := &conflicts.TestTx{
		BytesV: utils.RandomBytes(32),
		EpochV: priorEpoch,
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		TransitionV: trC,
	}
	vtxA1 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   priorEpoch,
		ParentsV: []avalanche.Vertex{gVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{txA1},
		BytesV:   utils.RandomBytes(32),
	}
	vtxA2 := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   currentEpoch,
		ParentsV: []avalanche.Vertex{gVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{txA2},
		BytesV:   utils.RandomBytes(32),
	}
	vtxB := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   priorEpoch,
		ParentsV: []avalanche.Vertex{gVtx},
		HeightV:  1,
		TxsV:     []conflicts.Tx{txB},
		BytesV:   utils.RandomBytes(32),
	}
	vtxC := &avalanche.TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		EpochV:   priorEpoch,
		ParentsV: []avalanche.Vertex{vtxA1, vtxB},
		HeightV:  2,
		TxsV:     []conflicts.Tx{txC},
		BytesV:   utils.RandomBytes(32),
	}

	// Tell engine to issue vtxA2 by sending push query from [vdr]
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		assert.Equal(t, b, vtxA2.Bytes())
		vtxA2.StatusV = choices.Processing
		txA2.StatusV = choices.Processing
		trA.StatusV = choices.Processing
		return vtxA2, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id != vtxA2.ID() {
			assert.FailNow(t, "Called Get for unexpected vtxID: %s", id)
		}
		return vtxA2, nil
	}
	err = te.PushQuery(vdr, 0, vtxA2.ID(), vtxA2.Bytes())
	assert.NoError(t, err)
	assert.True(t, te.Consensus.VertexIssued(vtxA2), "should have issued vtxA2")

	// Tell engine to issue vtxA1 by sending push query from [vdr]
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		assert.Equal(t, b, vtxA1.Bytes())
		vtxA1.StatusV = choices.Processing
		txA1.StatusV = choices.Processing
		trA.StatusV = choices.Processing
		return vtxA1, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id != vtxA1.ID() {
			assert.FailNow(t, "Called Get for unexpected vtxID: %s", id)
		}
		return vtxA1, nil
	}
	err = te.PushQuery(vdr, 1, vtxA1.ID(), vtxA1.Bytes())
	assert.NoError(t, err)
	assert.True(t, te.Consensus.VertexIssued(vtxA1), "should have issued vtxA1")

	// Attempt to issue vtxC, which will be blocked on the issuance
	// of vtxB containing its other transition dependency.
	manager.ParseF = func(b []byte) (avalanche.Vertex, error) {
		assert.Equal(t, b, vtxC.Bytes())
		vtxC.StatusV = choices.Processing
		txC.StatusV = choices.Processing
		trC.StatusV = choices.Processing
		return vtxC, nil
	}
	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id != vtxC.ID() {
			assert.FailNow(t, "Unexpectedly called Get for vtxID: %s", id)
		}
		return vtxC, nil
	}
	err = te.PushQuery(vdr, 2, vtxC.ID(), vtxC.Bytes())
	assert.NoError(t, err)
	assert.False(t, te.Consensus.VertexIssued(vtxC), "should not have issued vtxC")
	assert.True(t, te.pending.Contains(vtxC.ID()), "vtxC should have been in pending")

	manager.GetF = func(id ids.ID) (avalanche.Vertex, error) {
		if id != vtxA2.ID() {
			assert.FailNow(t, "Unexpectedly called Get for vtxID: %s", id)
		}
		return vtxA2, nil
	}
	// Record two consecutive Chits messages for vtxA2 for the first two polls
	err = te.Chits(vdr, 0, []ids.ID{vtxA2.ID()})
	assert.NoError(t, err, "unexpected error calling Chits with vtxA2")
	assert.Equal(t, choices.Processing, vtxA2.Status(), "expected vtxA2 to be Processing")

	err = te.Chits(vdr, 1, []ids.ID{vtxA2.ID()})
	assert.NoError(t, err, "unexpected error calling Chits with vtxA2")
	assert.Equal(t, choices.Accepted, vtxA2.Status(), "expected vtxA2 to be Processing")

	// // Since trA was accepted into epoch 2, vtxC (epoch 1) should be abandoned because its
	// // transaction txC has a dependency that will be fulfilled in epoch 2.
	assert.False(t, te.pending.Contains(vtxC.ID()), "vtxC should have been abandoned")
}
