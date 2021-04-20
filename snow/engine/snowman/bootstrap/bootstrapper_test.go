// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"gotest.tools/assert"

	"github.com/ava-labs/avalanchego/database/memdb"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/snow/engine/common"
	"github.com/ava-labs/avalanchego/snow/engine/common/queue"
	"github.com/ava-labs/avalanchego/snow/engine/snowman/block"
	"github.com/ava-labs/avalanchego/snow/validators"
	"github.com/ava-labs/avalanchego/utils/constants"
)

var (
	errUnknownBlock = errors.New("unknown block")
)

func newConfig(t *testing.T) (Config, ids.ShortID, *common.SenderTest, *block.TestVM) {
	ctx := snow.DefaultContextTest()

	peers := validators.NewSet()
	db := memdb.New()
	sender := &common.SenderTest{}
	vm := &block.TestVM{}

	sender.T = t
	vm.T = t

	sender.Default(true)
	vm.Default(true)

	isBootstrapped := false
	subnet := &common.SubnetTest{
		T:               t,
		IsBootstrappedF: func() bool { return isBootstrapped },
		BootstrappedF:   func(ids.ID) { isBootstrapped = true },
	}

	sender.CantGetAcceptedFrontier = false

	peer := ids.GenerateTestShortID()
	if err := peers.AddWeight(peer, 1); err != nil {
		t.Fatal(err)
	}

	blocker, _ := queue.New(db)

	commonConfig := common.Config{
		Ctx:        ctx,
		Validators: peers,
		Beacons:    peers,
		SampleK:    peers.Len(),
		Alpha:      peers.Weight()/2 + 1,
		Sender:     sender,
		Subnet:     subnet,
		Delay:      &common.DelayTest{},
	}
	return Config{
		Config:  commonConfig,
		Blocked: blocker,
		VM:      vm,
	}, peer, sender, vm
}

// Single node in the accepted frontier; no need to fecth parent
func TestBootstrapperSingleFrontier(t *testing.T) {
	config, _, _, vm := newConfig(t)

	blkID0 := ids.Empty.Prefix(0)
	blkID1 := ids.Empty.Prefix(1)

	blkBytes0 := []byte{0}
	blkBytes1 := []byte{1}

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID0,
			StatusV: choices.Accepted,
		},
		HeightV: 0,
		BytesV:  blkBytes0,
	}
	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID1,
			StatusV: choices.Processing,
		},
		ParentV: blk0,
		HeightV: 1,
		BytesV:  blkBytes1,
	}

	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blk0.ID(), nil }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blk0.ID(), blkID)
		return blk0, nil
	}

	finished := new(bool)
	bs := Bootstrapper{}
	err := bs.Initialize(
		config,
		func() error { *finished = true; return nil },
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	acceptedIDs := []ids.ID{blkID1}

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch blkID {
		case blkID1:
			return blk1, nil
		case blkID0:
			return blk0, nil
		default:
			t.Fatal(errUnknownBlock)
			panic(errUnknownBlock)
		}
	}
	vm.ParseBlockF = func(blkBytes []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(blkBytes, blkBytes1):
			return blk1, nil
		case bytes.Equal(blkBytes, blkBytes0):
			return blk0, nil
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}

	vm.CantBootstrapping = false
	vm.CantBootstrapped = false

	err = bs.ForceAccepted(acceptedIDs)
	switch {
	case err != nil: // should finish
		t.Fatal(err)
	case !*finished:
		t.Fatalf("Bootstrapping should have finished")
	case blk1.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	}
}

// Requests the unknown block and gets back a MultiPut with unexpected request ID.
// Requests again and gets response from unexpected peer.
// Requests again and gets an unexpected block.
// Requests again and gets the expected block.
func TestBootstrapperUnknownByzantineResponse(t *testing.T) {
	config, peerID, sender, vm := newConfig(t)

	blkID0 := ids.Empty.Prefix(0)
	blkID1 := ids.Empty.Prefix(1)
	blkID2 := ids.Empty.Prefix(2)

	blkBytes0 := []byte{0}
	blkBytes1 := []byte{1}
	blkBytes2 := []byte{2}

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID0,
			StatusV: choices.Accepted,
		},
		HeightV: 0,
		BytesV:  blkBytes0,
	}
	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID1,
			StatusV: choices.Unknown,
		},
		ParentV: blk0,
		HeightV: 1,
		BytesV:  blkBytes1,
	}
	blk2 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID2,
			StatusV: choices.Processing,
		},
		ParentV: blk1,
		HeightV: 2,
		BytesV:  blkBytes2,
	}

	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blk0.ID(), nil }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blk0.ID(), blkID)
		return blk0, nil
	}

	finished := new(bool)
	bs := Bootstrapper{}
	err := bs.Initialize(
		config,
		func() error { *finished = true; return nil },
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	acceptedIDs := []ids.ID{blkID2}

	parsedBlk1 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch blkID {
		case blkID0:
			return blk0, nil
		case blkID1:
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID2:
			return blk2, nil
		default:
			t.Fatal(errUnknownBlock)
			panic(errUnknownBlock)
		}
	}
	vm.ParseBlockF = func(blkBytes []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(blkBytes, blkBytes0):
			return blk0, nil
		case bytes.Equal(blkBytes, blkBytes1):
			blk1.StatusV = choices.Processing
			parsedBlk1 = true
			return blk1, nil
		case bytes.Equal(blkBytes, blkBytes2):
			return blk2, nil
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}

	requestID := new(uint32)
	sender.GetAncestorsF = func(vdr ids.ShortID, reqID uint32, vtxID ids.ID) {
		if vdr != peerID {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		switch {
		case vtxID == blkID1:
		default:
			t.Fatalf("should have requested blk1")
		}
		*requestID = reqID
	}
	vm.CantBootstrapping = false

	if err := bs.ForceAccepted(acceptedIDs); err != nil { // should request blk1
		t.Fatal(err)
	}

	oldReqID := *requestID
	if err := bs.MultiPut(peerID, *requestID+1, [][]byte{blkBytes1}); err != nil { // respond with wrong request ID
		t.Fatal(err)
	} else if oldReqID != *requestID {
		t.Fatal("should not have sent new request")
	}

	if err := bs.MultiPut(ids.ShortID{1, 2, 3}, *requestID, [][]byte{blkBytes1}); err != nil { // respond from wrong peer
		t.Fatal(err)
	} else if oldReqID != *requestID {
		t.Fatal("should not have sent new request")
	}

	if err := bs.MultiPut(peerID, *requestID, [][]byte{blkBytes0}); err != nil { // respond with wrong block
		t.Fatal(err)
	} else if oldReqID == *requestID {
		t.Fatal("should have sent new request")
	}

	vm.CantBootstrapped = false

	err = bs.MultiPut(peerID, *requestID, [][]byte{blkBytes1})
	switch {
	case err != nil: // respond with right block
		t.Fatal(err)
	case !*finished:
		t.Fatalf("Bootstrapping should have finished")
	case blk0.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk1.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk2.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	}
}

// There are multiple needed blocks and MultiPut returns one at a time
func TestBootstrapperPartialFetch(t *testing.T) {
	config, peerID, sender, vm := newConfig(t)

	blkID0 := ids.Empty.Prefix(0)
	blkID1 := ids.Empty.Prefix(1)
	blkID2 := ids.Empty.Prefix(2)
	blkID3 := ids.Empty.Prefix(3)

	blkBytes0 := []byte{0}
	blkBytes1 := []byte{1}
	blkBytes2 := []byte{2}
	blkBytes3 := []byte{3}

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID0,
			StatusV: choices.Accepted,
		},
		HeightV: 0,
		BytesV:  blkBytes0,
	}
	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID1,
			StatusV: choices.Unknown,
		},
		ParentV: blk0,
		HeightV: 1,
		BytesV:  blkBytes1,
	}
	blk2 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID2,
			StatusV: choices.Unknown,
		},
		ParentV: blk1,
		HeightV: 2,
		BytesV:  blkBytes2,
	}
	blk3 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID3,
			StatusV: choices.Processing,
		},
		ParentV: blk2,
		HeightV: 3,
		BytesV:  blkBytes3,
	}

	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blk0.ID(), nil }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blk0.ID(), blkID)
		return blk0, nil
	}

	finished := new(bool)
	bs := Bootstrapper{}
	err := bs.Initialize(
		config,
		func() error { *finished = true; return nil },
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	acceptedIDs := []ids.ID{blkID3}

	parsedBlk1 := false
	parsedBlk2 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch blkID {
		case blkID0:
			return blk0, nil
		case blkID1:
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID2:
			if parsedBlk2 {
				return blk2, nil
			}
			return nil, errUnknownBlock
		case blkID3:
			return blk3, nil
		default:
			t.Fatal(errUnknownBlock)
			panic(errUnknownBlock)
		}
	}
	vm.ParseBlockF = func(blkBytes []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(blkBytes, blkBytes0):
			return blk0, nil
		case bytes.Equal(blkBytes, blkBytes1):
			blk1.StatusV = choices.Processing
			parsedBlk1 = true
			return blk1, nil
		case bytes.Equal(blkBytes, blkBytes2):
			blk2.StatusV = choices.Processing
			parsedBlk2 = true
			return blk2, nil
		case bytes.Equal(blkBytes, blkBytes3):
			return blk3, nil
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}

	requestID := new(uint32)
	requested := ids.Empty
	sender.GetAncestorsF = func(vdr ids.ShortID, reqID uint32, vtxID ids.ID) {
		if vdr != peerID {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		switch vtxID {
		case blkID1, blkID2:
		default:
			t.Fatalf("should have requested blk1 or blk2")
		}
		*requestID = reqID
		requested = vtxID
	}

	vm.CantBootstrapping = false

	if err := bs.ForceAccepted(acceptedIDs); err != nil { // should request blk2
		t.Fatal(err)
	}

	if err := bs.MultiPut(peerID, *requestID, [][]byte{blkBytes2}); err != nil { // respond with blk2
		t.Fatal(err)
	} else if requested != blkID1 {
		t.Fatal("should have requested blk1")
	}

	vm.CantBootstrapped = false

	if err := bs.MultiPut(peerID, *requestID, [][]byte{blkBytes1}); err != nil { // respond with blk1
		t.Fatal(err)
	} else if requested != blkID1 {
		t.Fatal("should not have requested another block")
	}

	switch {
	case !*finished:
		t.Fatalf("Bootstrapping should have finished")
	case blk0.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk1.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk2.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	}
}

// There are multiple needed blocks and MultiPut returns all at once
func TestBootstrapperMultiPut(t *testing.T) {
	config, peerID, sender, vm := newConfig(t)

	blkID0 := ids.Empty.Prefix(0)
	blkID1 := ids.Empty.Prefix(1)
	blkID2 := ids.Empty.Prefix(2)
	blkID3 := ids.Empty.Prefix(3)

	blkBytes0 := []byte{0}
	blkBytes1 := []byte{1}
	blkBytes2 := []byte{2}
	blkBytes3 := []byte{3}

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID0,
			StatusV: choices.Accepted,
		},
		HeightV: 0,
		BytesV:  blkBytes0,
	}
	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID1,
			StatusV: choices.Unknown,
		},
		ParentV: blk0,
		HeightV: 1,
		BytesV:  blkBytes1,
	}
	blk2 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID2,
			StatusV: choices.Unknown,
		},
		ParentV: blk1,
		HeightV: 2,
		BytesV:  blkBytes2,
	}
	blk3 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID3,
			StatusV: choices.Processing,
		},
		ParentV: blk2,
		HeightV: 3,
		BytesV:  blkBytes3,
	}

	vm.CantBootstrapping = false
	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blk0.ID(), nil }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blk0.ID(), blkID)
		return blk0, nil
	}
	finished := new(bool)
	bs := Bootstrapper{}
	err := bs.Initialize(
		config,
		func() error { *finished = true; return nil },
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	acceptedIDs := []ids.ID{blkID3}

	parsedBlk1 := false
	parsedBlk2 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch blkID {
		case blkID0:
			return blk0, nil
		case blkID1:
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID2:
			if parsedBlk2 {
				return blk2, nil
			}
			return nil, errUnknownBlock
		case blkID3:
			return blk3, nil
		default:
			t.Fatal(errUnknownBlock)
			panic(errUnknownBlock)
		}
	}
	vm.ParseBlockF = func(blkBytes []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(blkBytes, blkBytes0):
			return blk0, nil
		case bytes.Equal(blkBytes, blkBytes1):
			blk1.StatusV = choices.Processing
			parsedBlk1 = true
			return blk1, nil
		case bytes.Equal(blkBytes, blkBytes2):
			blk2.StatusV = choices.Processing
			parsedBlk2 = true
			return blk2, nil
		case bytes.Equal(blkBytes, blkBytes3):
			return blk3, nil
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}

	requestID := new(uint32)
	requested := ids.Empty
	sender.GetAncestorsF = func(vdr ids.ShortID, reqID uint32, vtxID ids.ID) {
		if vdr != peerID {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		switch vtxID {
		case blkID1, blkID2:
		default:
			t.Fatalf("should have requested blk1 or blk2")
		}
		*requestID = reqID
		requested = vtxID
	}

	if err := bs.ForceAccepted(acceptedIDs); err != nil { // should request blk2
		t.Fatal(err)
	}

	vm.CantBootstrapped = false

	if err := bs.MultiPut(peerID, *requestID, [][]byte{blkBytes2, blkBytes1}); err != nil { // respond with blk2 and blk1
		t.Fatal(err)
	} else if requested != blkID2 {
		t.Fatal("should not have requested another block")
	}

	switch {
	case !*finished:
		t.Fatalf("Bootstrapping should have finished")
	case blk0.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk1.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk2.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	}
}

func TestBootstrapperAcceptedFrontier(t *testing.T) {
	config, _, _, vm := newConfig(t)

	blkID := ids.GenerateTestID()

	dummyBlk := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID,
			StatusV: choices.Accepted,
		},
		HeightV: 0,
		BytesV:  []byte{1, 2, 3},
	}
	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blkID, nil }
	vm.GetBlockF = func(bID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blkID, bID)
		return dummyBlk, nil
	}
	bs := Bootstrapper{}
	err := bs.Initialize(
		config,
		nil,
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := bs.CurrentAcceptedFrontier()
	if err != nil {
		t.Fatal(err)
	}

	if len(accepted) != 1 {
		t.Fatalf("Only one block should be accepted")
	}
	if accepted[0] != blkID {
		t.Fatalf("Blk should be accepted")
	}
}

func TestBootstrapperFilterAccepted(t *testing.T) {
	config, _, _, vm := newConfig(t)

	blkID0 := ids.GenerateTestID()
	blkID1 := ids.GenerateTestID()
	blkID2 := ids.GenerateTestID()

	blk0 := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     blkID0,
		StatusV: choices.Accepted,
	}}
	blk1 := &snowman.TestBlock{TestDecidable: choices.TestDecidable{
		IDV:     blkID1,
		StatusV: choices.Accepted,
	}}

	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blk1.ID(), nil }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blk1.ID(), blkID)
		return blk1, nil
	}

	bs := Bootstrapper{}
	err := bs.Initialize(
		config,
		nil,
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	blkIDs := []ids.ID{blkID0, blkID1, blkID2}
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch blkID {
		case blkID0:
			return blk0, nil
		case blkID1:
			return blk1, nil
		case blkID2:
			return nil, errUnknownBlock
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}
	vm.CantBootstrapping = false

	accepted := bs.FilterAccepted(blkIDs)
	acceptedSet := ids.Set{}
	acceptedSet.Add(accepted...)

	if acceptedSet.Len() != 2 {
		t.Fatalf("Two blocks should be accepted")
	}
	if !acceptedSet.Contains(blkID0) {
		t.Fatalf("Blk should be accepted")
	}
	if !acceptedSet.Contains(blkID1) {
		t.Fatalf("Blk should be accepted")
	}
	if acceptedSet.Contains(blkID2) {
		t.Fatalf("Blk shouldn't be accepted")
	}
}

func TestBootstrapperFinalized(t *testing.T) {
	config, peerID, sender, vm := newConfig(t)

	blkID0 := ids.Empty.Prefix(0)
	blkID1 := ids.Empty.Prefix(1)
	blkID2 := ids.Empty.Prefix(2)

	blkBytes0 := []byte{0}
	blkBytes1 := []byte{1}
	blkBytes2 := []byte{2}

	blk0 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID0,
			StatusV: choices.Accepted,
		},
		HeightV: 0,
		BytesV:  blkBytes0,
	}
	blk1 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID1,
			StatusV: choices.Unknown,
		},
		ParentV: blk0,
		HeightV: 1,
		BytesV:  blkBytes1,
	}
	blk2 := &snowman.TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     blkID2,
			StatusV: choices.Unknown,
		},
		ParentV: blk1,
		HeightV: 2,
		BytesV:  blkBytes2,
	}

	finished := new(bool)
	bs := Bootstrapper{}
	vm.CantLastAccepted = false
	vm.LastAcceptedF = func() (ids.ID, error) { return blk0.ID(), nil }
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		assert.Equal(t, blk0.ID(), blkID)
		return blk0, nil
	}
	err := bs.Initialize(
		config,
		func() error { *finished = true; return nil },
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	acceptedIDs := []ids.ID{blkID1, blkID2}

	parsedBlk1 := false
	parsedBlk2 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch blkID {
		case blkID0:
			return blk0, nil
		case blkID1:
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID2:
			if parsedBlk2 {
				return blk2, nil
			}
			return nil, errUnknownBlock
		default:
			t.Fatal(errUnknownBlock)
			panic(errUnknownBlock)
		}
	}
	vm.ParseBlockF = func(blkBytes []byte) (snowman.Block, error) {
		switch {
		case bytes.Equal(blkBytes, blkBytes0):
			return blk0, nil
		case bytes.Equal(blkBytes, blkBytes1):
			blk1.StatusV = choices.Processing
			parsedBlk1 = true
			return blk1, nil
		case bytes.Equal(blkBytes, blkBytes2):
			blk2.StatusV = choices.Processing
			parsedBlk2 = true
			return blk2, nil
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}

	requestIDs := map[ids.ID]uint32{}
	sender.GetAncestorsF = func(vdr ids.ShortID, reqID uint32, vtxID ids.ID) {
		if vdr != peerID {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		requestIDs[vtxID] = reqID
	}

	vm.CantBootstrapping = false

	if err := bs.ForceAccepted(acceptedIDs); err != nil { // should request blk0 and blk1
		t.Fatal(err)
	}

	reqID, ok := requestIDs[blkID2]
	if !ok {
		t.Fatalf("should have requested blk2")
	}

	vm.CantBootstrapped = false

	if err := bs.MultiPut(peerID, reqID, [][]byte{blkBytes2, blkBytes1}); err != nil {
		t.Fatal(err)
	}

	reqID, ok = requestIDs[blkID1]
	if !ok {
		t.Fatalf("should have requested blk1")
	}

	if err := bs.GetAncestorsFailed(peerID, reqID); err != nil {
		t.Fatal(err)
	}

	switch {
	case !*finished:
		t.Fatalf("Bootstrapping should have finished")
	case blk0.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk1.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	case blk2.Status() != choices.Accepted:
		t.Fatalf("Block should be accepted")
	}
}
