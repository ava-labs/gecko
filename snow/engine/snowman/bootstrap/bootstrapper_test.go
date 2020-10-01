// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

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
	vm.SaveBlockF = func(snowman.Block) error { return nil }

	sender.Default(true)
	vm.Default(true)

	sender.CantGetAcceptedFrontier = false

	peer := ids.GenerateTestShortID()
	peers.AddWeight(peer, 1)

	blocker, _ := queue.New(db)

	commonConfig := common.Config{
		Ctx:        ctx,
		Validators: peers,
		Beacons:    peers,
		SampleK:    int(peers.Weight()),
		Alpha:      uint64(peers.Len()/2 + 1),
		Sender:     sender,
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

	acceptedIDs := ids.Set{}
	acceptedIDs.Add(blkID1)

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blkID1):
			return blk1, nil
		case blkID.Equals(blkID0):
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

	if err := bs.ForceAccepted(acceptedIDs); err != nil { // should finish
		t.Fatal(err)
	} else if !*finished {
		t.Fatalf("Bootstrapping should have finished")
	} else if blk1.Status() != choices.Accepted {
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

	acceptedIDs := ids.Set{}
	acceptedIDs.Add(blkID2)

	parsedBlk1 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blkID0):
			return blk0, nil
		case blkID.Equals(blkID1):
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID.Equals(blkID2):
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
		if !vdr.Equals(peerID) {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		switch {
		case vtxID.Equals(blkID1):
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

	if err := bs.MultiPut(ids.NewShortID([20]byte{1, 2, 3}), *requestID, [][]byte{blkBytes1}); err != nil { // respond from wrong peer
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

	if err := bs.MultiPut(peerID, *requestID, [][]byte{blkBytes1}); err != nil { // respond with right block
		t.Fatal(err)
	} else if !*finished {
		t.Fatalf("Bootstrapping should have finished")
	} else if blk0.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk1.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk2.Status() != choices.Accepted {
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

	acceptedIDs := ids.Set{}
	acceptedIDs.Add(blkID3)

	parsedBlk1 := false
	parsedBlk2 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blkID0):
			return blk0, nil
		case blkID.Equals(blkID1):
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID.Equals(blkID2):
			if parsedBlk2 {
				return blk2, nil
			}
			return nil, errUnknownBlock
		case blkID.Equals(blkID3):
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
		if !vdr.Equals(peerID) {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		switch {
		case vtxID.Equals(blkID1), vtxID.Equals(blkID2):
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
	} else if !requested.Equals(blkID1) {
		t.Fatal("should have requested blk1")
	}

	vm.CantBootstrapped = false

	if err := bs.MultiPut(peerID, *requestID, [][]byte{blkBytes1}); err != nil { // respond with blk1
		t.Fatal(err)
	} else if !requested.Equals(blkID1) {
		t.Fatal("should not have requested another block")
	}

	if !*finished {
		t.Fatalf("Bootstrapping should have finished")
	} else if blk0.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk1.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk2.Status() != choices.Accepted {
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

	acceptedIDs := ids.Set{}
	acceptedIDs.Add(blkID3)

	parsedBlk1 := false
	parsedBlk2 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blkID0):
			return blk0, nil
		case blkID.Equals(blkID1):
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID.Equals(blkID2):
			if parsedBlk2 {
				return blk2, nil
			}
			return nil, errUnknownBlock
		case blkID.Equals(blkID3):
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
		if !vdr.Equals(peerID) {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		switch {
		case vtxID.Equals(blkID1), vtxID.Equals(blkID2):
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
	} else if !requested.Equals(blkID2) {
		t.Fatal("should not have requested another block")
	}

	if !*finished {
		t.Fatalf("Bootstrapping should have finished")
	} else if blk0.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk1.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk2.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	}
}

func TestBootstrapperAcceptedFrontier(t *testing.T) {
	config, _, _, vm := newConfig(t)

	blkID := ids.GenerateTestID()

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

	vm.LastAcceptedF = func() ids.ID { return blkID }

	accepted := bs.CurrentAcceptedFrontier()

	if accepted.Len() != 1 {
		t.Fatalf("Only one block should be accepted")
	}
	if !accepted.Contains(blkID) {
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

	blkIDs := ids.Set{}
	blkIDs.Add(
		blkID0,
		blkID1,
		blkID2,
	)

	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blkID0):
			return blk0, nil
		case blkID.Equals(blkID1):
			return blk1, nil
		case blkID.Equals(blkID2):
			return nil, errUnknownBlock
		}
		t.Fatal(errUnknownBlock)
		return nil, errUnknownBlock
	}
	vm.CantBootstrapping = false

	accepted := bs.FilterAccepted(blkIDs)

	if accepted.Len() != 2 {
		t.Fatalf("Two blocks should be accepted")
	}
	if !accepted.Contains(blkID0) {
		t.Fatalf("Blk should be accepted")
	}
	if !accepted.Contains(blkID1) {
		t.Fatalf("Blk should be accepted")
	}
	if accepted.Contains(blkID2) {
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
	err := bs.Initialize(
		config,
		func() error { *finished = true; return nil },
		fmt.Sprintf("%s_%s", constants.PlatformName, config.Ctx.ChainID),
		prometheus.NewRegistry(),
	)
	if err != nil {
		t.Fatal(err)
	}

	acceptedIDs := ids.Set{}
	acceptedIDs.Add(blkID1)
	acceptedIDs.Add(blkID2)

	parsedBlk1 := false
	parsedBlk2 := false
	vm.GetBlockF = func(blkID ids.ID) (snowman.Block, error) {
		switch {
		case blkID.Equals(blkID0):
			return blk0, nil
		case blkID.Equals(blkID1):
			if parsedBlk1 {
				return blk1, nil
			}
			return nil, errUnknownBlock
		case blkID.Equals(blkID2):
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

	requestIDs := map[[32]byte]uint32{}
	sender.GetAncestorsF = func(vdr ids.ShortID, reqID uint32, vtxID ids.ID) {
		if !vdr.Equals(peerID) {
			t.Fatalf("Should have requested block from %s, requested from %s", peerID, vdr)
		}
		requestIDs[vtxID.Key()] = reqID
	}

	vm.CantBootstrapping = false

	if err := bs.ForceAccepted(acceptedIDs); err != nil { // should request blk0 and blk1
		t.Fatal(err)
	}

	reqID, ok := requestIDs[blkID2.Key()]
	if !ok {
		t.Fatalf("should have requested blk2")
	}

	vm.CantBootstrapped = false

	if err := bs.MultiPut(peerID, reqID, [][]byte{blkBytes2, blkBytes1}); err != nil {
		t.Fatal(err)
	}

	reqID, ok = requestIDs[blkID1.Key()]
	if !ok {
		t.Fatalf("should have requested blk1")
	}

	if err := bs.GetAncestorsFailed(peerID, reqID); err != nil {
		t.Fatal(err)
	}

	if !*finished {
		t.Fatalf("Bootstrapping should have finished")
	} else if blk0.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk1.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	} else if blk2.Status() != choices.Accepted {
		t.Fatalf("Block should be accepted")
	}
}
