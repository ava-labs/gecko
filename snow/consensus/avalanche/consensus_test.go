// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avalanche

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowball"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm"
	"github.com/ava-labs/avalanchego/utils/constants"
)

var (
	Tests = []func(*testing.T, Factory){
		MetricsTest,
		ParamsTest,
		NumProcessingTest,
		AddTest,
		VertexIssuedTest,
		TxIssuedTest,
		VirtuousTest,
		VirtuousSkippedUpdateTest,
		VotingTest,
		IgnoreInvalidVotingTest,
		TransitiveVotingTest,
		SplitVotingTest,
		TransitiveRejectionTest,
		IsVirtuousTest,
		QuiesceTest,
		OrphansTest,
		ErrorOnVacuousAcceptTest,
		ErrorOnTxAcceptTest,
		ErrorOnVtxAcceptTest,
		ErrorOnVtxRejectTest,
		ErrorOnParentVtxRejectTest,
		ErrorOnTransitiveVtxRejectTest,
	}
)

func ConsensusTest(t *testing.T, factory Factory) {
	for _, test := range Tests {
		test(t, factory)
	}
}

func MetricsTest(t *testing.T, factory Factory) {
	ctx := snow.DefaultContextTest()

	{
		avl := factory.New()
		params := Parameters{
			Parameters: snowball.Parameters{
				Namespace:         fmt.Sprintf("%s_%s", constants.PlatformName, ctx.ChainID),
				Metrics:           prometheus.NewRegistry(),
				K:                 2,
				Alpha:             2,
				BetaVirtuous:      1,
				BetaRogue:         2,
				ConcurrentRepolls: 1,
				OptimalProcessing: 1,
			},
			Parents:   2,
			BatchSize: 1,
		}
		err := params.Metrics.Register(prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "vtx_processing",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if err := avl.Initialize(ctx, params, nil); err == nil {
			t.Fatalf("should have failed due to registering a duplicated statistic")
		}
	}
	{
		avl := factory.New()
		params := Parameters{
			Parameters: snowball.Parameters{
				Namespace:         fmt.Sprintf("%s_%s", constants.PlatformName, ctx.ChainID),
				Metrics:           prometheus.NewRegistry(),
				K:                 2,
				Alpha:             2,
				BetaVirtuous:      1,
				BetaRogue:         2,
				ConcurrentRepolls: 1,
				OptimalProcessing: 1,
			},
			Parents:   2,
			BatchSize: 1,
		}
		err := params.Metrics.Register(prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "vtx_accepted",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if err := avl.Initialize(ctx, params, nil); err == nil {
			t.Fatalf("should have failed due to registering a duplicated statistic")
		}
	}
	{
		avl := factory.New()
		params := Parameters{
			Parameters: snowball.Parameters{
				Namespace:         fmt.Sprintf("%s_%s", constants.PlatformName, ctx.ChainID),
				Metrics:           prometheus.NewRegistry(),
				K:                 2,
				Alpha:             2,
				BetaVirtuous:      1,
				BetaRogue:         2,
				ConcurrentRepolls: 1,
				OptimalProcessing: 1,
			},
			Parents:   2,
			BatchSize: 1,
		}
		err := params.Metrics.Register(prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: params.Namespace,
			Name:      "vtx_rejected",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if err := avl.Initialize(ctx, params, nil); err == nil {
			t.Fatalf("should have failed due to registering a duplicated statistic")
		}
	}
}

func ParamsTest(t *testing.T, factory Factory) {
	avl := factory.New()

	ctx := snow.DefaultContextTest()
	params := Parameters{
		Parameters: snowball.Parameters{
			Namespace:             fmt.Sprintf("%s_%s", constants.PlatformName, ctx.ChainID),
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}

	if err := avl.Initialize(ctx, params, nil); err != nil {
		t.Fatal(err)
	}

	p := avl.Parameters()
	switch {
	case p.K != params.K:
		t.Fatalf("Wrong K parameter")
	case p.Alpha != params.Alpha:
		t.Fatalf("Wrong Alpha parameter")
	case p.BetaVirtuous != params.BetaVirtuous:
		t.Fatalf("Wrong Beta1 parameter")
	case p.BetaRogue != params.BetaRogue:
		t.Fatalf("Wrong Beta2 parameter")
	case p.Parents != params.Parents:
		t.Fatalf("Wrong Parents parameter")
	}
}

func NumProcessingTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID()}

	if err := avl.Initialize(snow.DefaultContextTest(), params, vts); err != nil {
		t.Fatal(err)
	}

	if numProcessing := avl.NumProcessing(); numProcessing != 0 {
		t.Fatalf("expected %d vertices processing but returned %d", 0, numProcessing)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	}

	if numProcessing := avl.NumProcessing(); numProcessing != 1 {
		t.Fatalf("expected %d vertices processing but returned %d", 1, numProcessing)
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	}

	if numProcessing := avl.NumProcessing(); numProcessing != 2 {
		t.Fatalf("expected %d vertices processing but returned %d", 2, numProcessing)
	}

	if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	}

	if numProcessing := avl.NumProcessing(); numProcessing != 2 {
		t.Fatalf("expected %d vertices processing but returned %d", 2, numProcessing)
	}

	if err := avl.Add(vts[0]); err != nil {
		t.Fatal(err)
	}

	if numProcessing := avl.NumProcessing(); numProcessing != 2 {
		t.Fatalf("expected %d vertices processing but returned %d", 2, numProcessing)
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx0.ID())
	if err := avl.RecordPoll(votes); err != nil {
		t.Fatal(err)
	}

	if numProcessing := avl.NumProcessing(); numProcessing != 0 {
		t.Fatalf("expected %d vertices processing but returned %d", 0, numProcessing)
	}
}

func AddTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID()}

	if err := avl.Initialize(snow.DefaultContextTest(), params, vts); err != nil {
		t.Fatal(err)
	}

	if !avl.Finalized() {
		t.Fatalf("An empty avalanche instance is not finalized")
	} else if !ids.UnsortedEquals([]ids.ID{vts[0].ID(), vts[1].ID()}, avl.Preferences().List()) {
		t.Fatalf("Initial frontier failed to be set")
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	err := avl.Add(vtx0)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("A non-empty avalanche instance is finalized")
	case !ids.UnsortedEquals([]ids.ID{vtx0.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	err = avl.Add(vtx1)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("A non-empty avalanche instance is finalized")
	case !ids.UnsortedEquals([]ids.ID{vtx0.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	}

	err = avl.Add(vtx1)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("A non-empty avalanche instance is finalized")
	case !ids.UnsortedEquals([]ids.ID{vtx0.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	}

	err = avl.Add(vts[0])
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("A non-empty avalanche instance is finalized")
	case !ids.UnsortedEquals([]ids.ID{vtx0.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	}
}

func VertexIssuedTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID()}

	if err := avl.Initialize(snow.DefaultContextTest(), params, vts); err != nil {
		t.Fatal(err)
	}

	if !avl.VertexIssued(vts[0]) {
		t.Fatalf("Genesis Vertex not reported as issued")
	}

	tx := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx.InputIDsV = append(tx.InputIDsV, utxos[0])

	vtx := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx},
	}

	if avl.VertexIssued(vtx) {
		t.Fatalf("Vertex reported as issued")
	} else if err := avl.Add(vtx); err != nil {
		t.Fatal(err)
	} else if !avl.VertexIssued(vtx) {
		t.Fatalf("Vertex reported as not issued")
	}
}

func TxIssuedTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}
	vts := []Vertex{&TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		},
		TxsV: []snowstorm.Tx{tx0},
	}}
	utxos := []ids.ID{ids.GenerateTestID()}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	if err := avl.Initialize(snow.DefaultContextTest(), params, vts); err != nil {
		t.Fatal(err)
	}

	if !avl.TxIssued(tx0) {
		t.Fatalf("Genesis Tx not reported as issued")
	} else if avl.TxIssued(tx1) {
		t.Fatalf("Tx reported as issued")
	}

	vtx := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		HeightV: 1,
		TxsV:    []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx); err != nil {
		t.Fatal(err)
	} else if !avl.TxIssued(tx1) {
		t.Fatalf("Tx reported as not issued")
	}
}

func VirtuousTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          10,
			BetaRogue:             20,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	virtuous := avl.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(vts[0].ID()):
		t.Fatalf("Wrong virtuous")
	case !virtuous.Contains(vts[1].ID()):
		t.Fatalf("Wrong virtuous")
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	tx2 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx2.InputIDsV = append(tx2.InputIDsV, utxos[1])

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx0},
		HeightV:  2,
		TxsV:     []snowstorm.Tx{tx2},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if virtuous := avl.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(vtx0.IDV) {
		t.Fatalf("Wrong virtuous")
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if virtuous := avl.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(vtx0.IDV) {
		t.Fatalf("Wrong virtuous")
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx1.ID())
	votes.Add(1, vtx1.ID())

	if err := avl.RecordPoll(votes); err != nil {
		t.Fatal(err)
	}

	virtuous = avl.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(vts[0].ID()):
		t.Fatalf("Wrong virtuous")
	case !virtuous.Contains(vts[1].ID()):
		t.Fatalf("Wrong virtuous")
	}

	if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	}

	virtuous = avl.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(vts[0].ID()):
		t.Fatalf("Wrong virtuous")
	case !virtuous.Contains(vts[1].ID()):
		t.Fatalf("Wrong virtuous")
	}

	if err := avl.RecordPoll(votes); err != nil {
		t.Fatal(err)
	}

	virtuous = avl.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(vts[0].ID()):
		t.Fatalf("Wrong virtuous")
	case !virtuous.Contains(vts[1].ID()):
		t.Fatalf("Wrong virtuous")
	}
}

func VirtuousSkippedUpdateTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          10,
			BetaRogue:             20,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{
		ids.GenerateTestID(),
		ids.GenerateTestID(),
	}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	virtuous := avl.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(vts[0].ID()):
		t.Fatalf("Wrong virtuous")
	case !virtuous.Contains(vts[1].ID()):
		t.Fatalf("Wrong virtuous")
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if virtuous := avl.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(vtx0.IDV) {
		t.Fatalf("Wrong virtuous")
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if virtuous := avl.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(vtx0.IDV) {
		t.Fatalf("Wrong virtuous")
	} else if err := avl.RecordPoll(ids.UniqueBag{}); err != nil {
		t.Fatal(err)
	} else if virtuous := avl.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(vtx0.IDV) {
		t.Fatalf("Wrong virtuous")
	}
}

func VotingTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	}

	sm := ids.UniqueBag{}
	sm.Add(0, vtx1.IDV)
	sm.Add(1, vtx1.IDV)

	err = avl.RecordPoll(sm)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("An avalanche instance finalized too early")
	case !ids.UnsortedEquals([]ids.ID{vtx1.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	}

	err = avl.RecordPoll(sm)
	switch {
	case err != nil:
		t.Fatal(err)
	case !avl.Finalized():
		t.Fatalf("An avalanche instance finalized too late")
	case !ids.UnsortedEquals([]ids.ID{vtx1.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	case tx0.Status() != choices.Rejected:
		t.Fatalf("Tx should have been rejected")
	case tx1.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	}
}

func IgnoreInvalidVotingTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     3,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}

	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID()}

	if err := avl.Initialize(snow.DefaultContextTest(), params, vts); err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	}

	sm := ids.UniqueBag{}
	sm.Add(0, vtx0.IDV)
	sm.Add(1, vtx1.IDV)

	// Add Illegal Vote cast by Response 2
	sm.Add(2, vtx0.IDV)
	sm.Add(2, vtx1.IDV)

	if err := avl.RecordPoll(sm); err != nil {
		t.Fatal(err)
	} else if avl.Finalized() {
		t.Fatalf("An avalanche instance finalized too early")
	}
}

func TransitiveVotingTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[1])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx0},
		HeightV:  2,
		TxsV:     []snowstorm.Tx{tx1},
	}

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx1},
		HeightV:  3,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	}

	sm1 := ids.UniqueBag{}
	sm1.Add(0, vtx0.IDV)
	sm1.Add(1, vtx2.IDV)

	err = avl.RecordPoll(sm1)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("An avalanche instance finalized too early")
	case !ids.UnsortedEquals([]ids.ID{vtx2.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	case tx0.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	}

	sm2 := ids.UniqueBag{}
	sm2.Add(0, vtx2.IDV)
	sm2.Add(1, vtx2.IDV)

	err = avl.RecordPoll(sm2)
	switch {
	case err != nil:
		t.Fatal(err)
	case !avl.Finalized():
		t.Fatalf("An avalanche instance finalized too late")
	case !ids.UnsortedEquals([]ids.ID{vtx2.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	case tx0.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	case tx1.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	}
}

func SplitVotingTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	}

	sm1 := ids.UniqueBag{}
	sm1.Add(0, vtx0.IDV) // peer 0 votes for the tx though vtx0
	sm1.Add(1, vtx1.IDV) // peer 1 votes for the tx though vtx1

	err = avl.RecordPoll(sm1)
	switch {
	case err != nil:
		t.Fatal(err)
	case !avl.Finalized():
		t.Fatalf("An avalanche instance finalized too late")
	case !ids.UnsortedEquals([]ids.ID{vtx0.IDV, vtx1.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	case tx0.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	}
}

func TransitiveRejectionTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	tx2 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx2.InputIDsV = append(tx2.InputIDsV, utxos[1])

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx0},
		HeightV:  2,
		TxsV:     []snowstorm.Tx{tx2},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	}

	sm := ids.UniqueBag{}
	sm.Add(0, vtx1.IDV)
	sm.Add(1, vtx1.IDV)

	err = avl.RecordPoll(sm)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("An avalanche instance finalized too early")
	case !ids.UnsortedEquals([]ids.ID{vtx1.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	}

	err = avl.RecordPoll(sm)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("An avalanche instance finalized too early")
	case !ids.UnsortedEquals([]ids.ID{vtx1.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	case tx0.Status() != choices.Rejected:
		t.Fatalf("Tx should have been rejected")
	case tx1.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	case tx2.Status() != choices.Processing:
		t.Fatalf("Tx should not have been decided")
	}

	err = avl.RecordPoll(sm)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.Finalized():
		t.Fatalf("An avalanche instance finalized too early")
	case !ids.UnsortedEquals([]ids.ID{vtx1.IDV}, avl.Preferences().List()):
		t.Fatalf("Initial frontier failed to be set")
	case tx0.Status() != choices.Rejected:
		t.Fatalf("Tx should have been rejected")
	case tx1.Status() != choices.Accepted:
		t.Fatalf("Tx should have been accepted")
	case tx2.Status() != choices.Processing:
		t.Fatalf("Tx should not have been decided")
	}
}

func IsVirtuousTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     2,
			Alpha:                 2,
			BetaVirtuous:          1,
			BetaRogue:             2,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	virtuous := avl.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(vts[0].ID()):
		t.Fatalf("Wrong virtuous")
	case !virtuous.Contains(vts[1].ID()):
		t.Fatalf("Wrong virtuous")
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if !avl.IsVirtuous(tx0) {
		t.Fatalf("Should be virtuous.")
	} else if !avl.IsVirtuous(tx1) {
		t.Fatalf("Should be virtuous.")
	}

	err = avl.Add(vtx0)
	switch {
	case err != nil:
		t.Fatal(err)
	case !avl.IsVirtuous(tx0):
		t.Fatalf("Should be virtuous.")
	case avl.IsVirtuous(tx1):
		t.Fatalf("Should not be virtuous.")
	}

	err = avl.Add(vtx1)
	switch {
	case err != nil:
		t.Fatal(err)
	case avl.IsVirtuous(tx0):
		t.Fatalf("Should not be virtuous.")
	case avl.IsVirtuous(tx1):
		t.Fatalf("Should not be virtuous.")
	}
}

func QuiesceTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	tx2 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx2.InputIDsV = append(tx2.InputIDsV, utxos[1])

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx2},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if avl.Quiesce() {
		t.Fatalf("Shouldn't quiesce")
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if !avl.Quiesce() {
		t.Fatalf("Should quiesce")
	} else if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	} else if avl.Quiesce() {
		t.Fatalf("Shouldn't quiesce")
	}

	sm := ids.UniqueBag{}
	sm.Add(0, vtx2.IDV)
	if err := avl.RecordPoll(sm); err != nil {
		t.Fatal(err)
	} else if !avl.Quiesce() {
		t.Fatalf("Should quiesce")
	}
}

func OrphansTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          math.MaxInt32,
			BetaRogue:             math.MaxInt32,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
		&TestVertex{TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Accepted,
		}},
	}
	utxos := []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	tx2 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx2.InputIDsV = append(tx2.InputIDsV, utxos[1])

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx0},
		HeightV:  2,
		TxsV:     []snowstorm.Tx{tx2},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if orphans := avl.Orphans(); orphans.Len() != 0 {
		t.Fatalf("Wrong number of orphans")
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if orphans := avl.Orphans(); orphans.Len() != 0 {
		t.Fatalf("Wrong number of orphans")
	} else if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	} else if orphans := avl.Orphans(); orphans.Len() != 0 {
		t.Fatalf("Wrong number of orphans")
	}

	sm := ids.UniqueBag{}
	sm.Add(0, vtx1.IDV)
	if err := avl.RecordPoll(sm); err != nil {
		t.Fatal(err)
	} else if orphans := avl.Orphans(); orphans.Len() != 1 {
		t.Fatalf("Wrong number of orphans")
	} else if !orphans.Contains(tx2.ID()) {
		t.Fatalf("Wrong orphan")
	}
}

func ErrorOnVacuousAcceptTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          math.MaxInt32,
			BetaRogue:             math.MaxInt32,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{&TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		AcceptV: errors.New(""),
		StatusV: choices.Processing,
	}}

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	if err := avl.Add(vtx0); err == nil {
		t.Fatalf("Should have errored on vertex issuance")
	}
}

func ErrorOnTxAcceptTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{&TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		AcceptV: errors.New(""),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx0.IDV)
	if err := avl.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on vertex acceptance")
	}
}

func ErrorOnVtxAcceptTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{&TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			AcceptV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx0.IDV)
	if err := avl.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on vertex acceptance")
	}
}

func ErrorOnVtxRejectTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{&TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx0.IDV)
	if err := avl.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on vertex rejection")
	}
}

func ErrorOnParentVtxRejectTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{&TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx1},
		HeightV:  2,
		TxsV:     []snowstorm.Tx{tx1},
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx0.IDV)
	if err := avl.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on vertex rejection")
	}
}

func ErrorOnTransitiveVtxRejectTest(t *testing.T, factory Factory) {
	avl := factory.New()

	params := Parameters{
		Parameters: snowball.Parameters{
			Metrics:               prometheus.NewRegistry(),
			K:                     1,
			Alpha:                 1,
			BetaVirtuous:          1,
			BetaRogue:             1,
			ConcurrentRepolls:     1,
			OptimalProcessing:     1,
			MaxOutstandingItems:   1,
			MaxItemProcessingTime: 1,
		},
		Parents:   2,
		BatchSize: 1,
	}
	vts := []Vertex{&TestVertex{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Accepted,
	}}}
	utxos := []ids.ID{ids.GenerateTestID()}

	err := avl.Initialize(snow.DefaultContextTest(), params, vts)
	if err != nil {
		t.Fatal(err)
	}

	tx0 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx0.InputIDsV = append(tx0.InputIDsV, utxos[0])

	vtx0 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx0},
	}

	tx1 := &snowstorm.TestTx{TestDecidable: choices.TestDecidable{
		IDV:     ids.GenerateTestID(),
		StatusV: choices.Processing,
	}}
	tx1.InputIDsV = append(tx1.InputIDsV, utxos[0])

	vtx1 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			StatusV: choices.Processing,
		},
		ParentsV: vts,
		HeightV:  1,
		TxsV:     []snowstorm.Tx{tx1},
	}

	vtx2 := &TestVertex{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.GenerateTestID(),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		ParentsV: []Vertex{vtx1},
		HeightV:  1,
	}

	if err := avl.Add(vtx0); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx1); err != nil {
		t.Fatal(err)
	} else if err := avl.Add(vtx2); err != nil {
		t.Fatal(err)
	}

	votes := ids.UniqueBag{}
	votes.Add(0, vtx0.IDV)
	if err := avl.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on vertex rejection")
	}
}
