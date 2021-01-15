// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package snowstorm

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/stretchr/testify/assert"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowstorm/conflicts"

	sbcon "github.com/ava-labs/avalanchego/snow/consensus/snowball"
)

var (
	Tests = []func(*testing.T, Factory){
		MetricsTest,
		ParamsTest,
		IssuedTest,
		LeftoverInputTest,
		LowerConfidenceTest,
		MiddleConfidenceTest,
		IndependentTest,
		VirtuousTest,
		IsVirtuousTest,
		QuiesceTest,
		AcceptingDependencyTest,
		AcceptingSlowDependencyTest,
		RejectingDependencyTest,
		RejectingSlowDependencyTest,
		ConflictsTest,
		VirtuousDependsOnRogueTest,
		ErrorOnAcceptedTest,
		ErrorOnRejectingLowerConfidenceConflictTest,
		ErrorOnRejectingHigherConfidenceConflictTest,
		UTXOCleanupTest,
	}

	Red, Green, Blue, Alpha *conflicts.TestTx
)

//  R - G - B - A
func Setup() {
	Red = &conflicts.TestTx{TransitionV: &conflicts.TestTransition{}}
	Green = &conflicts.TestTx{TransitionV: &conflicts.TestTransition{}}
	Blue = &conflicts.TestTx{TransitionV: &conflicts.TestTransition{}}
	Alpha = &conflicts.TestTx{TransitionV: &conflicts.TestTransition{}}

	for i, color := range []*conflicts.TestTx{Red, Green, Blue, Alpha} {
		transitionIntf := color.Transition()
		transition := transitionIntf.(*conflicts.TestTransition)
		transition.IDV = ids.Empty.Prefix(uint64(i))
		transition.DependenciesV = nil
		transition.StatusV = choices.Processing

		color.IDV = transition.IDV.Prefix(0)
		color.AcceptV = nil
		color.RejectV = nil
		color.StatusV = choices.Processing

	}

	X := ids.Empty.Prefix(4)
	Y := ids.Empty.Prefix(5)
	Z := ids.Empty.Prefix(6)

	Red.Transition().(*conflicts.TestTransition).InputIDsV = []ids.ID{X}
	Green.Transition().(*conflicts.TestTransition).InputIDsV = []ids.ID{X, Y}
	Blue.Transition().(*conflicts.TestTransition).InputIDsV = []ids.ID{Y, Z}
	Alpha.Transition().(*conflicts.TestTransition).InputIDsV = []ids.ID{Z}
}

// Execute all tests against a consensus implementation
func ConsensusTest(t *testing.T, factory Factory, prefix string) {
	for i, test := range Tests {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			Setup()
			test(t, factory)
		})
	}
	t.Run("test-string", func(t *testing.T) {
		Setup()
		StringTest(t, factory, prefix)
	})
}

func MetricsTest(t *testing.T, factory Factory) {
	{
		params := sbcon.Parameters{
			Metrics:           prometheus.NewRegistry(),
			K:                 2,
			Alpha:             2,
			BetaVirtuous:      1,
			BetaRogue:         2,
			ConcurrentRepolls: 1,
		}
		err := params.Metrics.Register(prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tx_processing",
		}))
		if err != nil {
			t.Fatal(err)
		}
		graph := factory.New()
		if err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params); err == nil {
			t.Fatalf("should have errored due to a duplicated metric")
		}
	}
	{
		params := sbcon.Parameters{
			Metrics:           prometheus.NewRegistry(),
			K:                 2,
			Alpha:             2,
			BetaVirtuous:      1,
			BetaRogue:         2,
			ConcurrentRepolls: 1,
		}
		err := params.Metrics.Register(prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tx_accepted",
		}))
		if err != nil {
			t.Fatal(err)
		}
		graph := factory.New()
		if err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params); err == nil {
			t.Fatalf("should have errored due to a duplicated metric")
		}
	}
	{
		params := sbcon.Parameters{
			Metrics:           prometheus.NewRegistry(),
			K:                 2,
			Alpha:             2,
			BetaVirtuous:      1,
			BetaRogue:         2,
			ConcurrentRepolls: 1,
		}
		err := params.Metrics.Register(prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tx_rejected",
		}))
		if err != nil {
			t.Fatal(err)
		}
		graph := factory.New()
		if err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params); err == nil {
			t.Fatalf("should have errored due to a duplicated metric")
		}
	}
}

func ParamsTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	if p := graph.Parameters(); p.K != params.K {
		t.Fatalf("Wrong K parameter")
	} else if p := graph.Parameters(); p.Alpha != params.Alpha {
		t.Fatalf("Wrong Alpha parameter")
	} else if p := graph.Parameters(); p.BetaVirtuous != params.BetaVirtuous {
		t.Fatalf("Wrong Beta1 parameter")
	} else if p := graph.Parameters(); p.BetaRogue != params.BetaRogue {
		t.Fatalf("Wrong Beta2 parameter")
	}
}

func IssuedTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	if issued := graph.Issued(Red); issued {
		t.Fatalf("Haven't issued anything yet.")
	}
	graph.Add(Red)
	if issued := graph.Issued(Red); !issued {
		t.Fatalf("Have already issued.")
	}

	_ = Blue.Accept()

	if issued := graph.Issued(Blue); !issued {
		t.Fatalf("Have already accepted.")
	}
}

func LeftoverInputTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 1:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s got %s", Red.ID(), prefs.List()[0])
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	r := ids.Bag{}
	r.SetThreshold(2)
	r.AddCount(Red.ID(), 2)
	if updated, err := graph.RecordPoll(r); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case !graph.Finalized():
		t.Fatalf("Finalized too late")
	case Red.Status() != choices.Accepted:
		t.Fatalf("%s should have been accepted", Red.ID())
	case Green.Status() != choices.Rejected:
		t.Fatalf("%s should have been rejected", Green.ID())
	}
}

func LowerConfidenceTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(Blue)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 1:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s got %s", Red.ID(), prefs.List()[0])
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	r := ids.Bag{}
	r.SetThreshold(2)
	r.AddCount(Red.ID(), 2)
	if updated, err := graph.RecordPoll(r); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 1:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Blue.ID()):
		t.Fatalf("Wrong preference. Expected %s", Blue.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}
}

func MiddleConfidenceTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(Alpha)
	graph.Add(Blue)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(Alpha.ID()):
		t.Fatalf("Wrong preference. Expected %s", Alpha.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	r := ids.Bag{}
	r.SetThreshold(2)
	r.AddCount(Red.ID(), 2)
	if updated, err := graph.RecordPoll(r); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 1:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Alpha.ID()):
		t.Fatalf("Wrong preference. Expected %s", Alpha.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}
}

func IndependentTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      2,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Alpha)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(Alpha.ID()):
		t.Fatalf("Wrong preference. Expected %s", Alpha.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	ra := ids.Bag{}
	ra.SetThreshold(2)
	ra.AddCount(Red.ID(), 2)
	ra.AddCount(Alpha.ID(), 2)
	if updated, err := graph.RecordPoll(ra); err != nil {
		t.Fatal(err)
	} else if updated {
		t.Fatalf("Shouldn't have updated the frontiers")
	} else if prefs := graph.Preferences(); prefs.Len() != 2 {
		t.Fatalf("Wrong number of preferences.")
	} else if !prefs.Contains(Red.ID()) {
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	} else if !prefs.Contains(Alpha.ID()) {
		t.Fatalf("Wrong preference. Expected %s", Alpha.ID())
	} else if graph.Finalized() {
		t.Fatalf("Finalized too early")
	} else if updated, err := graph.RecordPoll(ra); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	} else if prefs := graph.Preferences(); prefs.Len() != 0 {
		t.Fatalf("Wrong number of preferences.")
	} else if !graph.Finalized() {
		t.Fatalf("Finalized too late")
	}
}

func VirtuousTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	if virtuous := graph.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(Red.ID()) {
		t.Fatalf("Wrong virtuous. Expected %s", Red.ID())
	}
	graph.Add(Alpha)

	virtuous := graph.Virtuous()
	switch {
	case virtuous.Len() != 2:
		t.Fatalf("Wrong number of virtuous.")
	case !virtuous.Contains(Red.ID()):
		t.Fatalf("Wrong virtuous. Expected %s", Red.ID())
	case !virtuous.Contains(Alpha.ID()):
		t.Fatalf("Wrong virtuous. Expected %s", Alpha.ID())
	}
	graph.Add(Green)
	if virtuous := graph.Virtuous(); virtuous.Len() != 1 {
		t.Fatalf("Wrong number of virtuous.")
	} else if !virtuous.Contains(Alpha.ID()) {
		t.Fatalf("Wrong virtuous. Expected %s", Alpha.ID())
	}
	graph.Add(Blue)
	if virtuous := graph.Virtuous(); virtuous.Len() != 0 {
		t.Fatalf("Wrong number of virtuous.")
	}
}

func IsVirtuousTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	if err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params); err != nil {
		t.Fatal(err)
	}

	if v, _ := graph.IsVirtuous(Red); !v {
		t.Fatalf("Should be virtuous")
	}
	if v, _ := graph.IsVirtuous(Green); !v {
		t.Fatalf("Should be virtuous")
	}
	if v, _ := graph.IsVirtuous(Blue); !v {
		t.Fatalf("Should be virtuous")
	}
	if v, _ := graph.IsVirtuous(Alpha); !v {
		t.Fatalf("Should be virtuous")
	}

	graph.Add(Red)
	if v, _ := graph.IsVirtuous(Red); !v {
		t.Fatalf("Should be virtuous")
	}
	if v, _ := graph.IsVirtuous(Green); v {
		t.Fatalf("Should not be virtuous")
	}
	if v, _ := graph.IsVirtuous(Blue); !v {
		t.Fatalf("Should be virtuous")
	}
	if v, _ := graph.IsVirtuous(Alpha); !v {
		t.Fatalf("Should be virtuous")
	}

	graph.Add(Green)
	if v, _ := graph.IsVirtuous(Red); v {
		t.Fatalf("Should not be virtuous")
	}
	if v, _ := graph.IsVirtuous(Green); v {
		t.Fatalf("Should not be virtuous")
	}
	if v, _ := graph.IsVirtuous(Blue); v {
		t.Fatalf("Should not be virtuous")
	}
}

func QuiesceTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	if !graph.Quiesce() {
		t.Fatalf("Should quiesce")
	}
	graph.Add(Red)
	if graph.Quiesce() {
		t.Fatalf("Shouldn't quiesce")
	}
	graph.Add(Green)
	if !graph.Quiesce() {
		t.Fatalf("Should quiesce")
	}
}

func AcceptingDependencyTest(t *testing.T, factory Factory) {
	graph := factory.New()

	purpleTransitionID := ids.Empty.Prefix(7)
	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     purpleTransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           purpleTransitionID,
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{Red.Transition()},
			InputIDsV:     []ids.ID{ids.Empty.Prefix(8)},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	if err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params); err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(purple)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	g := ids.Bag{}
	g.Add(Green.ID())
	if updated, err := graph.RecordPoll(g); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	rp := ids.Bag{}
	rp.Add(Red.ID(), purple.ID())
	if updated, err := graph.RecordPoll(rp); err != nil {
		t.Fatal(err)
	} else if updated {
		t.Fatalf("Shouldn't have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	r := ids.Bag{}
	r.Add(Red.ID())
	if updated, err := graph.RecordPoll(r); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case Red.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Accepted)
	case Green.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Rejected)
	case purple.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Accepted)
	}
}

func AcceptingSlowDependencyTest(t *testing.T, factory Factory) {
	graph := factory.New()

	purpleTransitionID := ids.Empty.Prefix(7)
	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     purpleTransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           purpleTransitionID,
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{Red.Transition()},
			InputIDsV:     []ids.ID{ids.Empty.Prefix(8)},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(purple)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	g := ids.Bag{}
	g.Add(Green.ID())
	if updated, err := graph.RecordPoll(g); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	p := ids.Bag{}
	p.Add(purple.ID())
	if updated, err := graph.RecordPoll(p); err != nil {
		t.Fatal(err)
	} else if updated {
		t.Fatalf("Shouldn't have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	rp := ids.Bag{}
	rp.Add(Red.ID(), purple.ID())
	if updated, err := graph.RecordPoll(rp); err != nil {
		t.Fatal(err)
	} else if updated {
		t.Fatalf("Shouldn't have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	r := ids.Bag{}
	r.Add(Red.ID())
	if updated, err := graph.RecordPoll(r); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case Red.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Accepted)
	case Green.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Rejected)
	case purple.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Accepted)
	}
}

func RejectingDependencyTest(t *testing.T, factory Factory) {
	graph := factory.New()

	purpleTransitionID := ids.Empty.Prefix(7)
	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     purpleTransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           purpleTransitionID,
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{Red.Transition(), Blue.Transition()},
			InputIDsV:     []ids.ID{ids.Empty.Prefix(8)},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(Blue)
	graph.Add(purple)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case Blue.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Blue.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	gp := ids.Bag{}
	gp.Add(Green.ID(), purple.ID())
	if updated, err := graph.RecordPoll(gp); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case Blue.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Blue.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	}

	if updated, err := graph.RecordPoll(gp); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case Red.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Rejected)
	case Green.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Accepted)
	case Blue.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", Blue.ID(), choices.Rejected)
	case purple.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Rejected)
	}
}

func RejectingSlowDependencyTest(t *testing.T, factory Factory) {
	graph := factory.New()

	purpleTransitionID := ids.Empty.Prefix(100)
	conflictID := ids.Empty.Prefix(101)
	cyanTransitionID := ids.Empty.Prefix(102)
	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     purpleTransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           purpleTransitionID,
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{Red.Transition()},
			InputIDsV:     []ids.ID{conflictID},
		},
	}
	cyan := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     cyanTransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       cyanTransitionID,
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{conflictID},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(purple)
	graph.Add(cyan)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(purple.ID()):
		t.Fatalf("Wrong preference. Expected %s", purple.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Processing)
	case cyan.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", cyan.ID(), choices.Processing)
	}

	c := ids.Bag{}
	c.Add(cyan.ID())
	if updated, err := graph.RecordPoll(c); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 1:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case Red.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Processing)
	case Green.Status() != choices.Processing:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Processing)
	case purple.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Rejected)
	case cyan.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", cyan.ID(), choices.Accepted)
	}

	g := ids.Bag{}
	g.Add(Green.ID())
	if updated, err := graph.RecordPoll(g); err != nil {
		t.Fatal(err)
	} else if !updated {
		t.Fatalf("Should have updated the frontiers")
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case Red.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", Red.ID(), choices.Rejected)
	case Green.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", Green.ID(), choices.Accepted)
	case purple.Status() != choices.Rejected:
		t.Fatalf("Wrong status. %s should be %s", purple.ID(), choices.Rejected)
	case cyan.Status() != choices.Accepted:
		t.Fatalf("Wrong status. %s should be %s", cyan.ID(), choices.Accepted)
	}
}

func ConflictsTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	conflictInputID := ids.Empty.Prefix(0)

	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(6),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{conflictInputID},
		},
	}

	orange := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(7),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{conflictInputID},
		},
	}

	graph.Add(purple)
	if orangeConflicts, _ := graph.Conflicts(orange); orangeConflicts.Len() != 1 {
		t.Fatalf("Wrong number of conflicts")
	} else if !orangeConflicts.Contains(purple.IDV) {
		t.Fatalf("Conflicts does not contain the right transaction")
	}
	graph.Add(orange)
	if orangeConflicts, _ := graph.Conflicts(orange); orangeConflicts.Len() != 1 {
		t.Fatalf("Wrong number of conflicts")
	} else if !orangeConflicts.Contains(purple.IDV) {
		t.Fatalf("Conflicts does not contain the right transaction")
	}
}

func VirtuousDependsOnRogueTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	rogue1TransitionID := ids.Empty.Prefix(0)
	rogue2TransitionID := ids.Empty.Prefix(1)
	virtuousTransitionID := ids.Empty.Prefix(2)
	input1 := ids.Empty.Prefix(3)
	input2 := ids.Empty.Prefix(4)

	rogue1 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     rogue1TransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       rogue1TransitionID,
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{input1},
		},
	}
	rogue2 := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     rogue2TransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       rogue2TransitionID,
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{input1},
		},
	}
	virtuous := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     virtuousTransitionID.Prefix(0),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:           virtuousTransitionID,
			StatusV:       choices.Processing,
			DependenciesV: []conflicts.Transition{rogue1.Transition()},
			InputIDsV:     []ids.ID{input2},
		},
	}

	graph.Add(rogue1)
	graph.Add(rogue2)
	graph.Add(virtuous)

	votes := ids.Bag{}
	votes.Add(rogue1.ID())
	votes.Add(virtuous.ID())
	if updated, err := graph.RecordPoll(votes); err != nil {
		t.Fatal(err)
	} else if updated {
		t.Fatalf("Shouldn't have updated the frontiers")
	} else if status := rogue1.Status(); status != choices.Processing {
		t.Fatalf("Rogue Tx is %s expected %s", status, choices.Processing)
	} else if status := rogue2.Status(); status != choices.Processing {
		t.Fatalf("Rogue Tx is %s expected %s", status, choices.Processing)
	} else if status := virtuous.Status(); status != choices.Processing {
		t.Fatalf("Virtuous Tx is %s expected %s", status, choices.Processing)
	} else if !graph.Quiesce() {
		t.Fatalf("Should quiesce as there are no pending virtuous transactions")
	}
}

func ErrorOnAcceptedTest(t *testing.T, factory Factory) {
	graph := factory.New()

	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(7),
			AcceptV: errors.New(""),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{ids.Empty.Prefix(4)},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(purple)

	votes := ids.Bag{}
	votes.Add(purple.ID())
	if _, err := graph.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on accepting an invalid tx")
	}
}

func ErrorOnRejectingLowerConfidenceConflictTest(t *testing.T, factory Factory) {
	graph := factory.New()

	X := ids.Empty.Prefix(4)

	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(7),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{X},
		},
	}

	pink := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(8),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{X},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(purple)
	graph.Add(pink)

	votes := ids.Bag{}
	votes.Add(purple.ID())
	if _, err := graph.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on rejecting an invalid tx")
	}
}

func ErrorOnRejectingHigherConfidenceConflictTest(t *testing.T, factory Factory) {
	graph := factory.New()

	X := ids.Empty.Prefix(4)

	purple := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(7),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{X},
		},
	}

	pink := &conflicts.TestTx{
		TestDecidable: choices.TestDecidable{
			IDV:     ids.Empty.Prefix(8),
			RejectV: errors.New(""),
			StatusV: choices.Processing,
		},
		TransitionV: &conflicts.TestTransition{
			IDV:       ids.GenerateTestID(),
			StatusV:   choices.Processing,
			InputIDsV: []ids.ID{X},
		},
	}

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         1,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(pink)
	graph.Add(purple)

	votes := ids.Bag{}
	votes.Add(purple.ID())
	if _, err := graph.RecordPoll(votes); err == nil {
		t.Fatalf("Should have errored on rejecting an invalid tx")
	}
}

func UTXOCleanupTest(t *testing.T, factory Factory) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 1,
		Alpha:             1,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	assert.NoError(t, err)

	graph.Add(Red)
	graph.Add(Green)

	redVotes := ids.Bag{}
	redVotes.Add(Red.ID())
	changed, err := graph.RecordPoll(redVotes)
	assert.NoError(t, err)
	assert.False(t, changed, "shouldn't have accepted the red tx")

	changed, err = graph.RecordPoll(redVotes)
	assert.NoError(t, err)
	assert.True(t, changed, "should have accepted the red tx")

	assert.Equal(t, choices.Accepted, Red.Status())
	assert.Equal(t, choices.Rejected, Green.Status())

	graph.Add(Blue)

	blueVotes := ids.Bag{}
	blueVotes.Add(Blue.ID())
	changed, err = graph.RecordPoll(blueVotes)
	assert.NoError(t, err)
	assert.True(t, changed, "should have accepted the blue tx")

	assert.Equal(t, choices.Accepted, Blue.Status())
}

func StringTest(t *testing.T, factory Factory, prefix string) {
	graph := factory.New()

	params := sbcon.Parameters{
		Metrics:           prometheus.NewRegistry(),
		K:                 2,
		Alpha:             2,
		BetaVirtuous:      1,
		BetaRogue:         2,
		ConcurrentRepolls: 1,
	}
	err := graph.Initialize(snow.DefaultContextTest(), conflicts.New(), params)
	if err != nil {
		t.Fatal(err)
	}

	graph.Add(Red)
	graph.Add(Green)
	graph.Add(Blue)
	graph.Add(Alpha)

	prefs := graph.Preferences()
	switch {
	case prefs.Len() != 1:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s got %s", Red.ID(), prefs.List()[0])
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	rb := ids.Bag{}
	rb.SetThreshold(2)
	rb.AddCount(Red.ID(), 2)
	rb.AddCount(Blue.ID(), 2)
	if changed, err := graph.RecordPoll(rb); err != nil {
		t.Fatal(err)
	} else if !changed {
		t.Fatalf("Should have caused the frontiers to recalculate")
	}
	graph.Add(Blue)

	{
		expected := prefix + "(\n" +
			"    Choice[0] = ID:  f3gVAZfDW3DjMnu2t1HWRwMQ4QmJg4Sm6nBhbsb6b7rVWFDdy SB(NumSuccessfulPolls = 1, Confidence = 1)\n" +
			"    Choice[1] = ID:  pxM84sbqbwLzS7T3iVCygbMFLroJYMcZGf4AVKk2fDNTJMLdb SB(NumSuccessfulPolls = 0, Confidence = 0)\n" +
			"    Choice[2] = ID: 24Sage1EvWyFFCGNuGxEPXZkh7ghyPLjHdAocBUADmeCAMhgCJ SB(NumSuccessfulPolls = 0, Confidence = 0)\n" +
			"    Choice[3] = ID: 2gaBa2iQAps2NdgJJYpPkFtiyzWwyRPJuRcRN3kytmHXydXnsy SB(NumSuccessfulPolls = 1, Confidence = 1)\n" +
			")"
		if str := graph.String(); str != expected {
			t.Fatalf("Expected %s, got %s", expected, str)
		}
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(Blue.ID()):
		t.Fatalf("Wrong preference. Expected %s", Blue.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	ga := ids.Bag{}
	ga.SetThreshold(2)
	ga.AddCount(Green.ID(), 2)
	ga.AddCount(Alpha.ID(), 2)
	if changed, err := graph.RecordPoll(ga); err != nil {
		t.Fatal(err)
	} else if changed {
		t.Fatalf("Shouldn't have caused the frontiers to recalculate")
	}

	{
		expected := prefix + "(\n" +
			"    Choice[0] = ID:  f3gVAZfDW3DjMnu2t1HWRwMQ4QmJg4Sm6nBhbsb6b7rVWFDdy SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			"    Choice[1] = ID:  pxM84sbqbwLzS7T3iVCygbMFLroJYMcZGf4AVKk2fDNTJMLdb SB(NumSuccessfulPolls = 1, Confidence = 1)\n" +
			"    Choice[2] = ID: 24Sage1EvWyFFCGNuGxEPXZkh7ghyPLjHdAocBUADmeCAMhgCJ SB(NumSuccessfulPolls = 1, Confidence = 1)\n" +
			"    Choice[3] = ID: 2gaBa2iQAps2NdgJJYpPkFtiyzWwyRPJuRcRN3kytmHXydXnsy SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			")"
		if str := graph.String(); str != expected {
			t.Fatalf("Expected %s, got %s", expected, str)
		}
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(Blue.ID()):
		t.Fatalf("Wrong preference. Expected %s", Blue.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	empty := ids.Bag{}
	if changed, err := graph.RecordPoll(empty); err != nil {
		t.Fatal(err)
	} else if changed {
		t.Fatalf("Shouldn't have caused the frontiers to recalculate")
	}

	{
		expected := prefix + "(\n" +
			"    Choice[0] = ID:  f3gVAZfDW3DjMnu2t1HWRwMQ4QmJg4Sm6nBhbsb6b7rVWFDdy SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			"    Choice[1] = ID:  pxM84sbqbwLzS7T3iVCygbMFLroJYMcZGf4AVKk2fDNTJMLdb SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			"    Choice[2] = ID: 24Sage1EvWyFFCGNuGxEPXZkh7ghyPLjHdAocBUADmeCAMhgCJ SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			"    Choice[3] = ID: 2gaBa2iQAps2NdgJJYpPkFtiyzWwyRPJuRcRN3kytmHXydXnsy SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			")"
		if str := graph.String(); str != expected {
			t.Fatalf("Expected %s, got %s", expected, str)
		}
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Red.ID()):
		t.Fatalf("Wrong preference. Expected %s", Red.ID())
	case !prefs.Contains(Blue.ID()):
		t.Fatalf("Wrong preference. Expected %s", Blue.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	if changed, err := graph.RecordPoll(ga); err != nil {
		t.Fatal(err)
	} else if !changed {
		t.Fatalf("Should have caused the frontiers to recalculate")
	}

	{
		expected := prefix + "(\n" +
			"    Choice[0] = ID:  f3gVAZfDW3DjMnu2t1HWRwMQ4QmJg4Sm6nBhbsb6b7rVWFDdy SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			"    Choice[1] = ID:  pxM84sbqbwLzS7T3iVCygbMFLroJYMcZGf4AVKk2fDNTJMLdb SB(NumSuccessfulPolls = 2, Confidence = 1)\n" +
			"    Choice[2] = ID: 24Sage1EvWyFFCGNuGxEPXZkh7ghyPLjHdAocBUADmeCAMhgCJ SB(NumSuccessfulPolls = 2, Confidence = 1)\n" +
			"    Choice[3] = ID: 2gaBa2iQAps2NdgJJYpPkFtiyzWwyRPJuRcRN3kytmHXydXnsy SB(NumSuccessfulPolls = 1, Confidence = 0)\n" +
			")"
		if str := graph.String(); str != expected {
			t.Fatalf("Expected %s, got %s", expected, str)
		}
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 2:
		t.Fatalf("Wrong number of preferences.")
	case !prefs.Contains(Green.ID()):
		t.Fatalf("Wrong preference. Expected %s", Green.ID())
	case !prefs.Contains(Alpha.ID()):
		t.Fatalf("Wrong preference. Expected %s", Alpha.ID())
	case graph.Finalized():
		t.Fatalf("Finalized too early")
	}

	if changed, err := graph.RecordPoll(ga); err != nil {
		t.Fatal(err)
	} else if !changed {
		t.Fatalf("Should have caused the frontiers to recalculate")
	}

	{
		expected := prefix + "()"
		if str := graph.String(); str != expected {
			t.Fatalf("Expected %s, got %s", expected, str)
		}
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case !graph.Finalized():
		t.Fatalf("Finalized too late")
	case Green.Status() != choices.Accepted:
		t.Fatalf("%s should have been accepted", Green.ID())
	case Alpha.Status() != choices.Accepted:
		t.Fatalf("%s should have been accepted", Alpha.ID())
	case Red.Status() != choices.Rejected:
		t.Fatalf("%s should have been rejected", Red.ID())
	case Blue.Status() != choices.Rejected:
		t.Fatalf("%s should have been rejected", Blue.ID())
	}

	if changed, err := graph.RecordPoll(rb); err != nil {
		t.Fatal(err)
	} else if changed {
		t.Fatalf("Shouldn't have caused the frontiers to recalculate")
	}

	{
		expected := prefix + "()"
		if str := graph.String(); str != expected {
			t.Fatalf("Expected %s, got %s", expected, str)
		}
	}

	prefs = graph.Preferences()
	switch {
	case prefs.Len() != 0:
		t.Fatalf("Wrong number of preferences.")
	case !graph.Finalized():
		t.Fatalf("Finalized too late")
	case Green.Status() != choices.Accepted:
		t.Fatalf("%s should have been accepted", Green.ID())
	case Alpha.Status() != choices.Accepted:
		t.Fatalf("%s should have been accepted", Alpha.ID())
	case Red.Status() != choices.Rejected:
		t.Fatalf("%s should have been rejected", Red.ID())
	case Blue.Status() != choices.Rejected:
		t.Fatalf("%s should have been rejected", Blue.ID())
	}
}
