// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"bytes"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/snow/validators"
)

var (
	errUnknownBlock = errors.New("unknown block")
	errUnknownBytes = errors.New("unknown bytes")

	Genesis = ids.GenerateTestID()
)

func setup(t *testing.T) (ids.ShortID, validators.Set, *common.SenderTest, *block.TestVM, *Transitive, snowman.Block) {
	config := DefaultConfig()

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	vals.AddWeight(vdr, 1)

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	vm := &block.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)
	vm.CantSetPreference = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     Genesis,
		StatusV: choices.Accepted,
	}}

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	sender.CantGetAcceptedFrontier = false

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		if !blkID.Equals(gBlk.ID()) {
			t.Fatalf("Wrong block requested")
		}
		return gBlk, nil
	}

	te := &Transitive{}
	te.Initialize(config)

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	vm.GetBlockF = nil
	vm.LastAcceptedF = nil
	sender.CantGetAcceptedFrontier = true

	return vdr, vals, sender, vm, te, gBlk
}

func TestEngineShutdown(t *testing.T) {
	_, _, _, vm, transitive, _ := setup(t)
	vmShutdownCalled := false
	vm.ShutdownF = func() error { vmShutdownCalled = true; return nil }
	vm.CantShutdown = false
	transitive.Shutdown()
	if !vmShutdownCalled {
		t.Fatal("Shutting down the Transitive did not shutdown the VM")
	}
}

func TestEngineAdd(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	if !te.Ctx.ChainID.Equals(ids.Empty) {
		t.Fatalf("Wrong chain ID")
	}

	parent := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Unknown,
	}}
	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: parent,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	asked := new(bool)
	reqID := new(uint32)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, blkID ids.ID) {
		*reqID = requestID
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		if !vdr.Equals(inVdr) {
			t.Fatalf("Asking wrong validator for block")
		}
		if !blkID.Equals(blk.Parent()) {
			t.Fatalf("Asking for wrong block")
		}
	}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		if !bytes.Equal(b, blk.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return blk, nil
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blk.ID()) || blkID.Equals(parent.ID()):
			return nil, errors.New("blk isn't accepted")
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatalf("unexpectedly asked to get block %s", blkID)
		return nil, errors.New("")
	}

	te.Put(vdr, 0, blk.ID(), blk.Bytes())

	vm.ParseBlockF = nil

	if !*asked {
		t.Fatalf("Didn't ask for a missing block")
	}

	if len(te.blocked) != 1 {
		t.Fatalf("Should have been blocking on request")
	}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) { return nil, errUnknownBytes }

	te.Put(vdr, *reqID, blk.Parent(), nil)

	vm.ParseBlockF = nil

	if len(te.blocked) != 0 {
		t.Fatalf("Should have finished blocking issue")
	}
}

func TestEngineQuery(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	blocked := new(bool)
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		*blocked = true
		if blkID.Equals(gBlk.ID()) {
			return gBlk, nil
		} else if blkID.Equals(blk.ID()) {
			return nil, errUnknownBlock
		}
		t.Fatalf("Wrong block requested")
		return nil, errUnknownBlock
	}

	asked := new(bool)
	getRequestID := new(uint32)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, blkID ids.ID) {
		*asked = true
		*getRequestID = requestID
		if !vdr.Equals(inVdr) {
			t.Fatalf("Asking wrong validator for block")
		}
		if !(blk.ID().Equals(blkID) || gBlk.ID().Equals(blkID)) {
			t.Fatalf("Asking for wrong block")
		}
	}

	te.PullQuery(vdr, 15, blk.ID())
	if !*blocked {
		t.Fatalf("Didn't request block")
	}
	if !*asked {
		t.Fatalf("Didn't request block from validator")
	}

	queried := new(bool)
	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
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
		if !blk.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}

	chitted := new(bool)
	sender.ChitsF = func(inVdr ids.ShortID, requestID uint32, prefSet ids.Set) {
		if *chitted {
			t.Fatalf("Sent multiple chits")
		}
		*chitted = true
		if requestID != 15 {
			t.Fatalf("Wrong request ID")
		}
		if prefSet.Len() != 1 {
			t.Fatal("Should only be one vote")
		}
		if !blk.ID().Equals(prefSet.List()[0]) {
			t.Fatalf("Wrong chits block")
		}
	}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		if !bytes.Equal(b, blk.Bytes()) {
			t.Fatalf("Wrong bytes")
		}
		return blk, nil
	}
	te.Put(vdr, *getRequestID, blk.ID(), blk.Bytes())
	vm.ParseBlockF = nil

	if !*queried {
		t.Fatalf("Didn't ask for preferences")
	}
	if !*chitted {
		t.Fatalf("Didn't provide preferences")
	}

	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk,
		HeightV: 2,
		BytesV:  []byte{5, 4, 3, 2, 1, 9},
	}

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blk.ID()):
			return nil, errUnknownBlock
		case blkID.Equals(blk1.ID()):
			return nil, errUnknownBlock
		}
		t.Fatalf("Wrong block requested")
		panic("Should have failed")
	}

	*asked = false
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, blkID ids.ID) {
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		*getRequestID = requestID
		if !vdr.Equals(inVdr) {
			t.Fatalf("Asking wrong validator for block")
		}
		if !blk1.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}
	vm.SaveBlockF = func(b snowman.Block) error {
		if !b.ID().Equals(blk1.ID()) && !b.ID().Equals(blk.ID()) {
			t.Fatal("should have asked to save blk or blk1")
		}
		return nil
	}

	blkSet := ids.Set{}
	blkSet.Add(blk1.ID())
	te.Chits(vdr, *queryRequestID, blkSet)

	*queried = false
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
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
		if !blkID.Equals(blk1.ID()) {
			t.Fatalf("Asking for wrong block")
		}
	}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		if !bytes.Equal(b, blk1.Bytes()) {
			t.Fatalf("Wrong bytes")
		}

		vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
			switch {
			case blkID.Equals(blk.ID()):
				return blk, nil
			case blkID.Equals(blk1.ID()):
				return blk1, nil
			}
			t.Fatalf("Wrong block requested")
			panic("Should have failed")
		}

		return blk1, nil
	}
	te.Put(vdr, *getRequestID, blk1.ID(), blk1.Bytes())
	vm.ParseBlockF = nil

	if blk1.Status() != choices.Accepted {
		t.Fatalf("Should have executed block")
	}
	if len(te.blocked) != 0 {
		t.Fatalf("Should have finished blocking")
	}

	_ = te.polls.String() // Shouldn't panic

	te.QueryFailed(vdr, *queryRequestID)
	if len(te.blocked) != 0 {
		t.Fatalf("Should have finished blocking")
	}
}

func TestEngineMultipleQuery(t *testing.T) {
	config := DefaultConfig()

	config.Params = snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 3,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}

	vals := validators.NewSet()
	config.Validators = vals

	vdr0 := ids.GenerateTestShortID()
	vdr1 := ids.GenerateTestShortID()
	vdr2 := ids.GenerateTestShortID()

	vals.AddWeight(vdr0, 1)
	vals.AddWeight(vdr1, 1)
	vals.AddWeight(vdr2, 1)

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	vm := &block.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)
	vm.CantSetPreference = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	sender.CantGetAcceptedFrontier = false

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		if !blkID.Equals(gBlk.ID()) {
			t.Fatalf("Wrong block requested")
		}
		return gBlk, nil
	}

	te := &Transitive{}
	te.Initialize(config)

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	vm.GetBlockF = nil
	vm.LastAcceptedF = nil
	sender.CantGetAcceptedFrontier = true

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	queried := new(bool)
	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
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
		if !blk0.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blk0.ID()):
			return nil, errors.New("")
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatalf("Wrong block requested")
		panic("Should have failed")
	}

	te.issue(blk0)

	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk0,
		HeightV: 2,
		BytesV:  []byte{2},
	}

	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		case id.Equals(blk0.ID()):
			return nil, errUnknownBlock
		case id.Equals(blk1.ID()):
			return nil, errUnknownBlock
		}
		t.Fatalf("Unknown block")
		panic("Should have errored")
	}

	asked := new(bool)
	getRequestID := new(uint32)
	sender.GetF = func(inVdr ids.ShortID, requestID uint32, blkID ids.ID) {
		if *asked {
			t.Fatalf("Asked multiple times")
		}
		*asked = true
		*getRequestID = requestID
		if !vdr0.Equals(inVdr) {
			t.Fatalf("Asking wrong validator for block")
		}
		if !blk1.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}
	vm.SaveBlockF = func(b snowman.Block) error {
		if b.ID().Equals(blk1.ID()) {
			return nil
		}
		t.Fatal("should have asked to save blk1")
		return errUnknownBlock
	}

	blkSet := ids.Set{}
	blkSet.Add(blk1.ID())
	te.Chits(vdr0, *queryRequestID, blkSet)
	te.Chits(vdr1, *queryRequestID, blkSet)

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
			switch {
			case blkID.Equals(blk0.ID()):
				return blk0, nil
			case blkID.Equals(blk1.ID()):
				return blk1, nil
			}
			t.Fatalf("Wrong block requested")
			panic("Should have failed")
		}

		return blk1, nil
	}

	*queried = false
	secondQueryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		*secondQueryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr0, vdr1, vdr2)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if !blk1.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}
	te.Put(vdr0, *getRequestID, blk1.ID(), blk1.Bytes())

	// Should be dropped because the query was already filled
	blkSet = ids.Set{}
	blkSet.Add(blk0.ID())
	te.Chits(vdr2, *queryRequestID, blkSet)

	if blk1.Status() != choices.Accepted {
		t.Fatalf("Should have executed block")
	}
	if len(te.blocked) != 0 {
		t.Fatalf("Should have finished blocking")
	}
}

func TestEngineBlockedIssue(t *testing.T) {
	_, _, sender, vm, te, gBlk := setup(t)

	sender.Default(false)

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk0,
		HeightV: 2,
		BytesV:  []byte{2},
	}
	sender.GetF = func(ids.ShortID, uint32, ids.ID) {}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		case blkID.Equals(blk1.ID()) || blkID.Equals(blk0.ID()):
			return nil, errUnknownBlock
		}
		t.Fatalf("Wrong block requested")
		panic("Should have failed")
	}

	te.issue(blk1)

	blk0.StatusV = choices.Processing
	te.issue(blk0)

	if !blk1.ID().Equals(te.Consensus.Preference()) {
		t.Fatalf("Should have issued blk1")
	}
}

func TestEngineAbandonResponse(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(false)

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		case blkID.Equals(blk.ID()):
			return nil, errUnknownBlock
		}
		t.Fatalf("Wrong block requested")
		panic("Should have failed")
	}

	te.issue(blk)
	te.QueryFailed(vdr, 1)

	if len(te.blocked) != 0 {
		t.Fatalf("Should have removed blocking event")
	}
}

func TestEngineFetchBlock(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(false)

	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		if id.Equals(gBlk.ID()) {
			return gBlk, nil
		}
		t.Fatalf("Unknown block")
		panic("Should have failed")
	}

	added := new(bool)
	sender.PutF = func(inVdr ids.ShortID, requestID uint32, blkID ids.ID, blk []byte) {
		if !vdr.Equals(inVdr) {
			t.Fatalf("Wrong validator")
		}
		if requestID != 123 {
			t.Fatalf("Wrong request id")
		}
		if !gBlk.ID().Equals(blkID) {
			t.Fatalf("Wrong blockID")
		}
		*added = true
	}

	te.Get(vdr, 123, gBlk.ID())

	if !*added {
		t.Fatalf("Should have sent block to peer")
	}
}

func TestEnginePushQuery(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		if bytes.Equal(b, blk.Bytes()) {
			return blk, nil
		}
		return nil, errUnknownBytes
	}

	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		if id.Equals(blk.ID()) {
			return nil, errUnknownBlock
		} else if id.Equals(gBlk.ID()) {
			return gBlk, nil
		}
		t.Fatal(errUnknownBytes)
		panic(errUnknownBytes)
	}

	chitted := new(bool)
	sender.ChitsF = func(inVdr ids.ShortID, requestID uint32, votes ids.Set) {
		if *chitted {
			t.Fatalf("Sent chit multiple times")
		}
		*chitted = true
		if !inVdr.Equals(vdr) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if requestID != 20 {
			t.Fatalf("Wrong request id")
		}
		if votes.Len() != 1 {
			t.Fatal("votes should only have one element")
		}
		vote := votes.List()[0]
		if !blk.ID().Equals(vote) {
			t.Fatalf("Asking for wrong block")
		}
	}

	queried := new(bool)
	sender.PushQueryF = func(inVdrs ids.ShortSet, _ uint32, blkID ids.ID, blkBytes []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if !blk.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}

	te.PushQuery(vdr, 20, blk.ID(), blk.Bytes())

	if !*chitted {
		t.Fatalf("Should have sent a chit to the peer")
	}
	if !*queried {
		t.Fatalf("Should have sent a query to the peer")
	}
}

func TestEngineBuildBlock(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		case blkID.Equals(blk.ID()):
			return nil, errors.New("")
		}
		t.Fatalf("Wrong block requested")
		panic("Should have failed")
	}

	queried := new(bool)
	sender.PushQueryF = func(inVdrs ids.ShortSet, _ uint32, blkID ids.ID, blkBytes []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
	}

	vm.BuildBlockF = func() (snowman.Block, error) { return blk, nil }
	te.Notify(common.PendingTxs)

	if !*queried {
		t.Fatalf("Should have sent a query to the peer")
	}
}

func TestEngineRepoll(t *testing.T) {
	vdr, _, sender, _, te, _ := setup(t)

	sender.Default(true)

	queried := new(bool)
	sender.PullQueryF = func(inVdrs ids.ShortSet, _ uint32, blkID ids.ID) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
	}

	te.repoll()

	if !*queried {
		t.Fatalf("Should have sent a query to the peer")
	}
}

func TestVoteCanceling(t *testing.T) {
	config := DefaultConfig()

	config.Params = snowball.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 3,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}

	vals := validators.NewSet()
	config.Validators = vals

	vdr0 := ids.GenerateTestShortID()
	vdr1 := ids.GenerateTestShortID()
	vdr2 := ids.GenerateTestShortID()

	vals.AddWeight(vdr0, 1)
	vals.AddWeight(vdr1, 1)
	vals.AddWeight(vdr2, 1)

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	vm := &block.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)
	vm.CantSetPreference = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		default:
			t.Fatalf("Loaded unknown block")
			panic("Should have failed")
		}
	}
	sender.CantGetAcceptedFrontier = false

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	te.Initialize(config)

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	vm.LastAcceptedF = nil
	sender.CantGetAcceptedFrontier = true

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	queried := new(bool)
	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
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
		if !blk.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}

	te.issue(blk)

	if te.polls.Len() != 1 {
		t.Fatalf("Shouldn't have finished blocking issue")
	}

	te.QueryFailed(vdr0, *queryRequestID)

	if te.polls.Len() != 1 {
		t.Fatalf("Shouldn't have finished blocking issue")
	}

	repolled := new(bool)
	sender.PullQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID) {
		*repolled = true
	}
	te.QueryFailed(vdr1, *queryRequestID)

	if !*repolled {
		t.Fatalf("Should have finished blocking issue and repolled the network")
	}
}

func TestEngineNoQuery(t *testing.T) {
	config := DefaultConfig()

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vm := &block.TestVM{}
	vm.T = t
	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }

	config.VM = vm
	te := &Transitive{}
	te.Initialize(config)
	te.finishBootstrapping()
	te.Ctx.Bootstrapped()

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	te.issue(blk)
}

func TestEngineNoRepollQuery(t *testing.T) {
	config := DefaultConfig()

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)
	sender.CantGetAcceptedFrontier = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vm := &block.TestVM{}
	vm.T = t
	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }

	config.VM = vm
	te := &Transitive{}
	te.Initialize(config)
	te.finishBootstrapping()
	te.Ctx.Bootstrapped()

	te.repoll()
}

func TestEngineAbandonQuery(t *testing.T) {
	vdr, _, sender, vm, te, _ := setup(t)

	sender.Default(true)

	blkID := ids.GenerateTestID()

	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(blkID):
			return nil, errUnknownBlock
		default:
			t.Fatalf("Loaded unknown block")
			panic("Should have failed")
		}
	}

	reqID := new(uint32)
	sender.GetF = func(_ ids.ShortID, requestID uint32, _ ids.ID) {
		*reqID = requestID
	}

	te.PullQuery(vdr, 0, blkID)

	if len(te.blocked) != 1 {
		t.Fatalf("Should have blocked on request")
	}

	te.GetFailed(vdr, *reqID)

	if len(te.blocked) != 0 {
		t.Fatalf("Should have removed request")
	}
}

func TestEngineAbandonChit(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		case blkID.Equals(blk.ID()):
			return nil, errors.New("")
		}
		t.Fatalf("Wrong block requested")
		panic("Should have failed")
	}

	sender.CantPushQuery = false

	te.issue(blk)

	fakeBlkID := ids.GenerateTestID()
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(fakeBlkID):
			return nil, errUnknownBlock
		default:
			t.Fatalf("Loaded unknown block")
			panic("Should have failed")
		}
	}

	reqID := new(uint32)
	sender.GetF = func(_ ids.ShortID, requestID uint32, _ ids.ID) {
		*reqID = requestID
	}

	fakeBlkIDSet := ids.Set{}
	fakeBlkIDSet.Add(fakeBlkID)
	te.Chits(vdr, 0, fakeBlkIDSet)

	if len(te.blocked) != 1 {
		t.Fatalf("Should have blocked on request")
	}

	te.GetFailed(vdr, *reqID)

	if len(te.blocked) != 0 {
		t.Fatalf("Should have removed request")
	}
}

func TestEngineBlockingChitRequest(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	missingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	parentBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: missingBlk,
		HeightV: 2,
		BytesV:  []byte{2},
	}
	blockingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: parentBlk,
		HeightV: 3,
		BytesV:  []byte{3},
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blockingBlk.ID()) || blkID.Equals(parentBlk.ID()) || blkID.Equals(missingBlk.ID()):
			return nil, errors.New("")
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		default:
			t.Fatalf("Loaded unknown block")
			panic("Should have failed")
		}
	}
	sender.GetF = func(ids.ShortID, uint32, ids.ID) {}

	te.issue(parentBlk)

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, blockingBlk.Bytes()):
			return blockingBlk, nil
		default:
			t.Fatalf("Loaded unknown block")
			panic("Should have failed")
		}
	}

	te.PushQuery(vdr, 0, blockingBlk.ID(), blockingBlk.Bytes())

	if len(te.blocked) != 3 {
		t.Fatalf("Both inserts should be blocking in addition to the chit request")
	}

	sender.CantPushQuery = false
	sender.CantChits = false

	missingBlk.StatusV = choices.Processing
	te.issue(missingBlk)

	if len(te.blocked) != 0 {
		t.Fatalf("Both inserts should not longer be blocking")
	}
}

func TestEngineBlockingChitResponse(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	issuedBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	missingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{2},
	}
	blockingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: missingBlk,
		HeightV: 2,
		BytesV:  []byte{3},
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blockingBlk.ID()) || blkID.Equals(issuedBlk.ID()) || blkID.Equals(missingBlk.ID()):
			return nil, errors.New("")
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		default:
			t.Fatalf("Loaded unknown block")
			panic("Should have failed")
		}
	}
	sender.GetF = func(ids.ShortID, uint32, ids.ID) {}

	te.issue(blockingBlk)

	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if !blkID.Equals(issuedBlk.ID()) {
			t.Fatalf("Asking for wrong block")
		}
	}

	te.issue(issuedBlk)

	blockingBlkIDSet := ids.Set{}
	blockingBlkIDSet.Add(blockingBlk.ID())
	te.Chits(vdr, *queryRequestID, blockingBlkIDSet)

	if len(te.blocked) != 2 {
		t.Fatalf("The insert and the chit should be blocking")
	}

	sender.PushQueryF = nil
	sender.CantPushQuery = false

	missingBlk.StatusV = choices.Processing
	te.issue(missingBlk)
}

func TestEngineRetryFetch(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	missingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	vm.CantGetBlock = false

	reqID := new(uint32)
	sender.GetF = func(_ ids.ShortID, requestID uint32, _ ids.ID) {
		*reqID = requestID
	}

	te.PullQuery(vdr, 0, missingBlk.ID())

	vm.CantGetBlock = true
	sender.GetF = nil

	te.GetFailed(vdr, *reqID)

	vm.CantGetBlock = false

	called := new(bool)
	sender.GetF = func(ids.ShortID, uint32, ids.ID) {
		*called = true
	}

	te.PullQuery(vdr, 0, missingBlk.ID())

	vm.CantGetBlock = true
	sender.GetF = nil

	if !*called {
		t.Fatalf("Should have requested the block again")
	}
}

func TestEngineUndeclaredDependencyDeadlock(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	validBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	invalidBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: validBlk,
		HeightV: 2,
		VerifyV: errors.New(""),
		BytesV:  []byte{2},
	}

	invalidBlkID := invalidBlk.ID()

	reqID := new(uint32)
	sender.PushQueryF = func(_ ids.ShortSet, requestID uint32, _ ids.ID, _ []byte) {
		*reqID = requestID
	}
	sender.PullQueryF = func(_ ids.ShortSet, requestID uint32, _ ids.ID) {}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, validBlk.Bytes()):
			return validBlk, nil
		case bytes.Equal(b, invalidBlk.Bytes()):
			return invalidBlk, nil
		}
		return nil, errUnknownBlock
	}
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(validBlk.ID()):
			return nil, errors.New("")
		case id.Equals(invalidBlk.ID()):
			return nil, errors.New("")
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatal("asked to get unexpected block")
		return nil, errUnknownBlock
	}
	sender.ChitsF = func(ids.ShortID, uint32, ids.Set) {}

	te.PushQuery(vdr, 0, validBlk.ID(), validBlk.Bytes())

	sender.PushQueryF = nil

	te.PushQuery(vdr, 1, invalidBlk.ID(), invalidBlk.Bytes())

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		return nil, errUnknownBlock
	}
	vm.SaveBlockF = func(b snowman.Block) error {
		if !b.ID().Equals(validBlk.ID()) {
			t.Fatal("should have saved validBlk")
		}
		return nil
	}

	votes := ids.Set{}
	votes.Add(invalidBlkID)
	te.Chits(vdr, *reqID, votes)

	vm.GetBlockF = nil

	if status := validBlk.Status(); status != choices.Accepted {
		t.Fatalf("Should have bubbled invalid votes to the valid parent")
	}
}

func TestEngineGossip(t *testing.T) {
	_, _, sender, vm, te, gBlk := setup(t)

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}

	called := new(bool)
	sender.GossipF = func(blkID ids.ID, blkBytes []byte) {
		*called = true
		switch {
		case !blkID.Equals(gBlk.ID()):
			t.Fatal(errUnknownBlock)
		}
		switch {
		case !bytes.Equal(blkBytes, gBlk.Bytes()):
			t.Fatal(errUnknownBytes)
		}
	}

	te.Gossip()

	if !*called {
		t.Fatalf("Should have gossiped the block")
	}
}

func TestEngineInvalidBlockIgnoredFromUnexpectedPeer(t *testing.T) {
	vdr, vdrs, sender, vm, te, gBlk := setup(t)

	secondVdr := ids.GenerateTestShortID()
	vdrs.AddWeight(secondVdr, 1)

	sender.Default(true)

	missingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	pendingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: missingBlk,
		HeightV: 2,
		BytesV:  []byte{2},
	}

	parsed := new(bool)
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, pendingBlk.Bytes()):
			*parsed = true
			return pendingBlk, nil
		}
		return nil, errUnknownBlock
	}

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		if !*parsed {
			return nil, errUnknownBlock
		}

		switch {
		case blkID.Equals(pendingBlk.ID()) || blkID.Equals(missingBlk.ID()):
			return nil, errors.New("")
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatal("GetBlock called with wrong block")
		return nil, errUnknownBlock
	}

	reqID := new(uint32)
	sender.GetF = func(reqVdr ids.ShortID, requestID uint32, blkID ids.ID) {
		*reqID = requestID
		if !reqVdr.Equals(vdr) {
			t.Fatalf("Wrong validator requested")
		}
		if !blkID.Equals(missingBlk.ID()) {
			t.Fatalf("Wrong block requested")
		}
	}

	te.PushQuery(vdr, 0, pendingBlk.ID(), pendingBlk.Bytes())

	te.Put(secondVdr, *reqID, missingBlk.ID(), []byte{3})

	*parsed = false
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, missingBlk.Bytes()):
			*parsed = true
			return missingBlk, nil
		}
		return nil, errUnknownBlock
	}
	sender.CantPushQuery = false
	sender.CantChits = false

	missingBlk.StatusV = choices.Processing

	te.Put(vdr, *reqID, missingBlk.ID(), missingBlk.Bytes())

	pref := te.Consensus.Preference()
	if !pref.Equals(pendingBlk.ID()) {
		t.Fatalf("Shouldn't have abandoned the pending block")
	}
}

func TestEnginePushQueryRequestIDConflict(t *testing.T) {
	vdr, _, sender, vm, te, gBlk := setup(t)

	sender.Default(true)

	missingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	pendingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: missingBlk,
		HeightV: 2,
		BytesV:  []byte{2},
	}

	randomBlkID := ids.GenerateTestID()

	parsed := new(bool)
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, pendingBlk.Bytes()):
			*parsed = true
			return pendingBlk, nil
		}
		return nil, errUnknownBlock
	}

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		return nil, errUnknownBlock
	}

	reqID := new(uint32)
	sender.GetF = func(reqVdr ids.ShortID, requestID uint32, blkID ids.ID) {
		*reqID = requestID
		if !reqVdr.Equals(vdr) {
			t.Fatalf("Wrong validator requested")
		}
		if !blkID.Equals(missingBlk.ID()) {
			t.Fatalf("Wrong block requested")
		}
	}

	te.PushQuery(vdr, 0, pendingBlk.ID(), pendingBlk.Bytes())

	sender.GetF = nil
	sender.CantGet = false

	te.PushQuery(vdr, *reqID, randomBlkID, []byte{3})

	*parsed = false
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, missingBlk.Bytes()):
			*parsed = true
			return missingBlk, nil
		}
		return nil, errUnknownBlock
	}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		if !*parsed {
			return nil, errUnknownBlock
		}

		switch {
		case blkID.Equals(missingBlk.ID()):
			return missingBlk, nil
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		}
		return nil, errUnknownBlock
	}
	sender.CantPushQuery = false
	sender.CantChits = false

	te.Put(vdr, *reqID, missingBlk.ID(), missingBlk.Bytes())

	pref := te.Consensus.Preference()
	if !pref.Equals(pendingBlk.ID()) {
		t.Fatalf("Shouldn't have abandoned the pending block")
	}
}

func TestEngineAggressivePolling(t *testing.T) {
	config := DefaultConfig()

	config.Params.ConcurrentRepolls = 2

	vals := validators.NewSet()
	config.Validators = vals

	vdr := ids.GenerateTestShortID()
	vals.AddWeight(vdr, 1)

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	vm := &block.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)
	vm.CantSetPreference = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	sender.CantGetAcceptedFrontier = false

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		if !blkID.Equals(gBlk.ID()) {
			t.Fatalf("Wrong block requested")
		}
		return gBlk, nil
	}

	te := &Transitive{}
	te.Initialize(config)

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	vm.GetBlockF = nil
	vm.LastAcceptedF = nil
	sender.CantGetAcceptedFrontier = true

	sender.Default(true)

	pendingBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	parsed := new(bool)
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, pendingBlk.Bytes()):
			*parsed = true
			return pendingBlk, nil
		}
		return nil, errUnknownBlock
	}

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		if !*parsed {
			return nil, errUnknownBlock
		}

		switch {
		case blkID.Equals(pendingBlk.ID()):
			return nil, errors.New("")
		case blkID.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatal("called GetBlock for wrong block ID")
		return nil, errUnknownBlock
	}

	numPushed := new(int)
	sender.PushQueryF = func(_ ids.ShortSet, _ uint32, _ ids.ID, _ []byte) { *numPushed++ }

	numPulled := new(int)
	sender.PullQueryF = func(_ ids.ShortSet, _ uint32, _ ids.ID) { *numPulled++ }

	te.Put(vdr, 0, pendingBlk.ID(), pendingBlk.Bytes())

	if *numPushed != 1 {
		t.Fatalf("Should have initially sent a push query")
	}

	if *numPulled != 1 {
		t.Fatalf("Should have sent an additional pull query")
	}
}

func TestEngineDoubleChit(t *testing.T) {
	config := DefaultConfig()

	config.Params = snowball.Parameters{
		Metrics:      prometheus.NewRegistry(),
		K:            2,
		Alpha:        2,
		BetaVirtuous: 1,
		BetaRogue:    2,
	}

	vals := validators.NewSet()
	config.Validators = vals

	vdr0 := ids.GenerateTestShortID()
	vdr1 := ids.GenerateTestShortID()

	vals.AddWeight(vdr0, 1)
	vals.AddWeight(vdr1, 1)

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	vm := &block.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)
	vm.CantSetPreference = false

	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	sender.CantGetAcceptedFrontier = false

	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		}
		t.Fatalf("Unknown block")
		panic("Should have errored")
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	te := &Transitive{}
	te.Initialize(config)

	vm.CantBootstrapping = true
	vm.CantBootstrapped = true

	vm.LastAcceptedF = nil
	sender.CantGetAcceptedFrontier = true

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, blk.Bytes()):
			return blk, nil
		}
		t.Fatalf("asked to parse unknown block")
		panic("Should have errored")
	}

	queried := new(bool)
	queryRequestID := new(uint32)
	sender.PushQueryF = func(inVdrs ids.ShortSet, requestID uint32, blkID ids.ID, blkBytes []byte) {
		if *queried {
			t.Fatalf("Asked multiple times")
		}
		*queried = true
		*queryRequestID = requestID
		vdrSet := ids.ShortSet{}
		vdrSet.Add(vdr0, vdr1)
		if !inVdrs.Equals(vdrSet) {
			t.Fatalf("Asking wrong validator for preference")
		}
		if !blk.ID().Equals(blkID) {
			t.Fatalf("Asking for wrong block")
		}
	}
	sender.ChitsF = func(ids.ShortID, uint32, ids.Set) {}

	te.PushQuery(vdr0, 0, blk.ID(), blk.Bytes())

	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		case id.Equals(blk.ID()):
			return nil, errUnknownBlock
		}
		t.Fatalf("Unknown block")
		panic("Should have errored")
	}

	blkSet := ids.Set{}
	blkSet.Add(blk.ID())

	if status := blk.Status(); status != choices.Processing {
		t.Fatalf("Wrong status: %s ; expected: %s", status, choices.Processing)
	}

	te.Chits(vdr0, *queryRequestID, blkSet)

	if status := blk.Status(); status != choices.Processing {
		t.Fatalf("Wrong status: %s ; expected: %s", status, choices.Processing)
	}

	te.Chits(vdr0, *queryRequestID, blkSet)

	if status := blk.Status(); status != choices.Processing {
		t.Fatalf("Wrong status: %s ; expected: %s", status, choices.Processing)
	}

	vm.SaveBlockF = func(b snowman.Block) error {
		if b.ID().Equals(blk.ID()) {
			return nil
		}
		t.Fatal("should have saved blk")
		return errUnknownBlock
	}

	te.Chits(vdr1, *queryRequestID, blkSet)

	if status := blk.Status(); status != choices.Accepted {
		t.Fatalf("Wrong status: %s ; expected: %s", status, choices.Accepted)
	}
}

// test that processing blocks are properly pinned/unpinned in memory
func TestPinnedMemory(t *testing.T) {
	// Do setup
	config := DefaultConfig()

	config.Params.ConcurrentRepolls = 0

	vdr := validators.GenerateRandomValidator(1)

	vals := validators.NewSet()
	config.Validators = vals

	vals.AddWeight(vdr.ID(), 1)

	sender := &common.SenderTest{}
	sender.T = t
	config.Sender = sender

	sender.Default(true)

	vm := &block.TestVM{}
	vm.T = t
	config.VM = vm

	vm.Default(true)
	vm.CantSetPreference = false

	// Genesis block
	gBlk := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}

	blk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: gBlk,
		HeightV: 1,
		BytesV:  []byte{1},
	}

	// The VM should return that it hasn't accepted [blk]
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		case id.Equals(blk.ID()):
			return nil, errors.New("unknown block")
		}
		t.Fatalf("Unknown block")
		panic("Should have errored")
	}
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, blk.Bytes()):
			return blk, nil
		}
		return nil, errUnknownBlock
	}

	// Key: Block ID
	// Value: ID of request for push query containing that block ID
	pushQueryIDs := map[[32]byte]uint32{}
	sender.PushQueryF = func(vdrID ids.ShortSet, requestID uint32, blkID ids.ID, blk []byte) {
		pushQueryIDs[blkID.Key()] = requestID
	}

	sender.ChitsF = func(ids.ShortID, uint32, ids.Set) {}

	vm.LastAcceptedF = func() ids.ID { return gBlk.ID() }
	sender.CantGetAcceptedFrontier = false

	te := &Transitive{}
	te.Initialize(config)
	te.Initialize(config)
	te.finishBootstrapping()
	te.Ctx.Bootstrapped()

	// First, test the simple case where there's one processing block with no conflicts and it's accepted
	// Put a block
	if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}
	te.Put(vdr.ID(), 0, blk.ID(), blk.Bytes())
	if len(te.processing) != 1 {
		t.Fatalf("processing should have %d elements but has %d", 1, len(te.processing))
	}
	// The VM should return that it has accepted [blk]
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(gBlk.ID()):
			return gBlk, nil
		case id.Equals(blk.ID()):
			return nil, errUnknownBlock
		}
		t.Fatalf("asked VM for unexpected block")
		panic("Should have errored")
	}
	vm.SaveBlockF = func(b snowman.Block) error {
		bID := b.ID()
		switch {
		case bID.Equals(blk.ID()):
			return nil
		}
		t.Fatal("asked to save wrong block")
		return errUnknownBlock
	}

	// Record chits
	votes := ids.Set{}
	votes.Add(blk.ID())
	reqID, ok := pushQueryIDs[blk.ID().Key()]
	if !ok {
		t.Fatal("didn't ask for blk")
	}
	te.Chits(vdr.ID(), reqID, votes)
	if blk.Status() != choices.Accepted {
		t.Fatal("should be accepted")
	} else if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}

	// Put the same block again
	te.Put(vdr.ID(), 1, blk.ID(), blk.Bytes())
	if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}
	// PushQuery for the same block
	te.PushQuery(vdr.ID(), 2, blk.ID(), blk.Bytes())
	if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}

	// Current tree:
	//       G
	//       |
	//      blk

	// Second, test the case where there are conflicting blocks and blocks with children

	// Add three blocks.
	blk2 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk,
		HeightV: 2,
		BytesV:  []byte{2},
	}
	blk3 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk2,
		HeightV: 3,
		BytesV:  []byte{3},
	}
	blk4 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk,
		HeightV: 2,
		BytesV:  []byte{4},
	}
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, blk2.Bytes()):
			return blk2, nil
		case bytes.Equal(b, blk3.Bytes()):
			return blk3, nil
		case bytes.Equal(b, blk4.Bytes()):
			return blk4, nil
		default:
			t.Fatal("asked for unexpected block")
		}
		return nil, errUnknownBlock
	}

	// Issue push queries for the new blocks
	te.PushQuery(vdr.ID(), 3, blk2.ID(), blk2.Bytes())
	if len(te.processing) != 1 {
		t.Fatalf("processing should have %d elements but has %d", 1, len(te.processing))
	}
	te.PushQuery(vdr.ID(), 4, blk3.ID(), blk3.Bytes())
	if len(te.processing) != 2 {
		t.Fatalf("processing should have %d elements but has %d", 2, len(te.processing))
	}
	te.PushQuery(vdr.ID(), 5, blk4.ID(), blk4.Bytes())
	if len(te.processing) != 3 {
		t.Fatalf("processing should have %d elements but has %d", 3, len(te.processing))
	}

	// Current tree:
	//       G
	//       |
	//      blk
	//     /   \
	//   blk2   blk4
	//    |
	//   blk3

	// Send 2 chits for blk3. Note that since blk2 conflicts with blk4,
	// we need 2 consecutive polls because betaRouge = 2
	votes.Clear()
	votes.Add(blk3.ID())
	// we could use any two push query request IDs here. We use blk2 and blk3's
	reqID, ok = pushQueryIDs[blk3.ID().Key()]
	if !ok {
		t.Fatal("should have sent push query for blk3")
	}
	vm.SaveBlockF = func(b snowman.Block) error {
		bID := b.ID()
		switch {
		case bID.Equals(blk2.ID()):
			return nil
		case bID.Equals(blk3.ID()):
			return nil
		}
		t.Fatal("asked to save wrong block")
		return errUnknownBlock
	}
	te.Chits(vdr.ID(), reqID, votes)
	reqID, ok = pushQueryIDs[blk2.ID().Key()]
	if !ok {
		t.Fatal("should have sent push query for blk2")
	}
	te.Chits(vdr.ID(), reqID, votes)
	// Should have accepted blk2 and blk3, rejected blk4
	if blk2.Status() != choices.Accepted {
		t.Fatal("should have accepted blk2")
	} else if blk3.Status() != choices.Accepted {
		t.Fatal("should have accepted blk3")
	} else if blk4.Status() != choices.Rejected {
		t.Fatal("should have rejected blk4")
	} else if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}

	// Push queries for these blocks should not add elements to processing
	// PushQuery for the same block
	te.PushQuery(vdr.ID(), 6, blk2.ID(), blk2.Bytes())
	te.PushQuery(vdr.ID(), 7, blk3.ID(), blk3.Bytes())
	te.PushQuery(vdr.ID(), 8, blk3.ID(), blk4.Bytes())
	if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}

	// Third, test the case where a block is immediately rejected because a parent is rejected
	blk5 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk4,
		HeightV: 3,
		BytesV:  []byte{5},
	}
	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, blk5.Bytes()):
			return blk5, nil
		default:
			t.Fatal("asked for unexpected block")
		}
		return nil, errUnknownBlock
	}
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(blk5.ID()):
			return nil, errors.New("unknown block")
		case id.Equals(blk4.ID()):
			return nil, errors.New("unknown block")
		}
		t.Fatalf("asked VM for unexpected block")
		panic("Should have errored")
	}
	te.PushQuery(vdr.ID(), 9, blk5.ID(), blk5.Bytes()) // Will ask for blk4 since we no longer hold it since it's rejected
	if blk5.Status() != choices.Rejected {
		t.Fatal("should be rejected")
	} else if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}

	// Fourth, test the case where a block is dropped
	blk6 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Unknown,
		},
		ParentV: blk3,
		HeightV: 4,
		BytesV:  []byte{6},
		VerifyV: errors.New("fail on verification"),
	}
	blk7 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentV: blk6,
		HeightV: 5,
		BytesV:  []byte{7},
	}

	vm.ParseBlockF = func(b []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(b, blk6.Bytes()):
			return blk6, nil
		case bytes.Equal(b, blk7.Bytes()):
			return blk7, nil
		default:
			t.Fatal("asked for unexpected block")
		}
		return nil, errUnknownBlock
	}
	vm.GetBlockF = func(id ids.ID) (snowman.Block, error) {
		switch {
		case id.Equals(blk3.ID()):
			return blk3, nil
		case id.Equals(blk6.ID()):
			return nil, errors.New("unknown block")
		case id.Equals(blk7.ID()):
			return nil, errors.New("unknown block")
		}
		t.Fatalf("asked VM for unexpected block")
		panic("Should have errored")
	}
	reqID = 0
	requested := false
	sender.GetF = func(vdr ids.ShortID, requestID uint32, blkID ids.ID) {
		if !blkID.Equals(blk6.ID()) {
			t.Fatal("should've asked for blk6")
		}
		requested = true
		reqID = requestID
	}
	te.PushQuery(vdr.ID(), 10, blk7.ID(), blk7.Bytes()) // Will block on blk6
	if !requested {
		t.Fatal("should have requested blk6")
	} else if len(te.processing) != 1 {
		t.Fatalf("processing should have %d elements but has %d", 1, len(te.processing))
	}
	te.Put(vdr.ID(), reqID, blk6.ID(), blk6.Bytes())

	// Current tree:
	//       G
	//       |
	//      blk
	//     /   \
	//   blk2   blk4 (rej.)
	//    |
	//   blk3
	//    |
	//   blk6
	//    |
	//   blk7

	// blk7 becomes unblocked. blk6 fails verification, so blk7 gets dropped.
	if len(te.processing) != 0 {
		t.Fatalf("processing should have %d elements but has %d", 0, len(te.processing))
	}
}
