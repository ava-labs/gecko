// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowman

import (
	"math/rand"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
	"github.com/ava-labs/avalanchego/utils/sampler"
)

type Network struct {
	params         snowball.Parameters
	colors         []*TestBlock
	nodes, running []Consensus
}

func (n *Network) shuffleColors() {
	s := sampler.NewUniform()
	_ = s.Initialize(uint64(len(n.colors)))
	indices, _ := s.Sample(len(n.colors))
	colors := []*TestBlock(nil)
	for _, index := range indices {
		colors = append(colors, n.colors[int(index)])
	}
	n.colors = colors
	SortTestBlocks(n.colors)
}

func (n *Network) Initialize(params snowball.Parameters, numColors int) {
	n.params = params
	// #nosec G404
	n.colors = append(n.colors, &TestBlock{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(uint64(rand.Int63())),
			StatusV: choices.Processing,
		},
		ParentV: Genesis,
		HeightV: 0,
	})

	for i := 1; i < numColors; i++ {
		dependency := n.colors[rand.Intn(len(n.colors))] // #nosec G404
		// #nosec G404
		n.colors = append(n.colors, &TestBlock{
			TestDecidable: choices.TestDecidable{
				IDV:     ids.Empty.Prefix(uint64(rand.Int63())),
				StatusV: choices.Processing,
			},
			ParentV: dependency,
			HeightV: dependency.HeightV + 1,
		})
	}
}

func (n *Network) AddNode(sm Consensus) error {
	n.params.Metrics = prometheus.NewRegistry()
	if err := sm.Initialize(snow.DefaultContextTest(), n.params, Genesis.ID(), Genesis.Height()); err != nil {
		return err
	}

	n.shuffleColors()
	deps := map[ids.ID]Block{}
	for _, blk := range n.colors {
		myDep, found := deps[blk.ParentV.ID()]
		if !found {
			myDep = blk.Parent()
		}
		myVtx := &TestBlock{
			TestDecidable: choices.TestDecidable{
				IDV:     blk.ID(),
				StatusV: blk.Status(),
			},
			ParentV: myDep,
			HeightV: blk.Height(),
			VerifyV: blk.Verify(),
			BytesV:  blk.Bytes(),
		}
		if err := sm.Add(myVtx); err != nil {
			return err
		}
		deps[myVtx.ID()] = myDep
	}
	n.nodes = append(n.nodes, sm)
	n.running = append(n.running, sm)
	return nil
}

func (n *Network) Finalized() bool { return len(n.running) == 0 }

func (n *Network) Round() error {
	if len(n.running) == 0 {
		return nil
	}

	runningInd := rand.Intn(len(n.running)) // #nosec G404
	running := n.running[runningInd]

	s := sampler.NewUniform()
	_ = s.Initialize(uint64(len(n.nodes)))
	indices, _ := s.Sample(n.params.K)
	sampledColors := ids.Bag{}
	for _, index := range indices {
		peer := n.nodes[int(index)]
		sampledColors.Add(peer.Preference())
	}

	if err := running.RecordPoll(sampledColors); err != nil {
		return err
	}

	// If this node has been finalized, remove it from the poller
	if running.Finalized() {
		newSize := len(n.running) - 1
		n.running[runningInd] = n.running[newSize]
		n.running = n.running[:newSize]
	}

	return nil
}

func (n *Network) Agreement() bool {
	if len(n.nodes) == 0 {
		return true
	}
	pref := n.nodes[0].Preference()
	for _, node := range n.nodes {
		if pref != node.Preference() {
			return false
		}
	}
	return true
}
