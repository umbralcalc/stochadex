package simulator_test

import (
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// randomWalkIteration is a minimal stochastic iteration whose RNG is built in
// Configure from the partition seed — the shape every framework iteration has,
// and the reason re-evaluating a model needs reseeding to be reproducible.
type randomWalkIteration struct{ rng *rand.Rand }

func (w *randomWalkIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	w.rng = rand.New(rand.NewPCG(settings.Iterations[partitionIndex].Seed, 0xabcd))
}

func (w *randomWalkIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	row := stateHistories[partitionIndex].Values.RawRowView(0)
	return []float64{row[0] + w.rng.NormFloat64() + params.GetIndex("drift", 0)}
}

func newTestReentrantSimulation(t *testing.T) *simulator.ReentrantSimulation {
	t.Helper()
	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "walk",
		Iteration:         &randomWalkIteration{},
		Params:            simulator.NewParams(map[string][]float64{"drift": {0}}),
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              1,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}
	return simulator.NewReentrantSimulation(settings, impl)
}

// TestReentrantSimulationIsPure is the property the type exists to provide, and
// the one a plain coordinator cannot give: evaluating the same model from the
// same state under the same seed must repeat, no matter what ran in between.
func TestReentrantSimulationIsPure(t *testing.T) {
	simulation := newTestReentrantSimulation(t)
	from := [][]float64{{5.0}}

	first := simulation.Advance(from, 42, 1)[0][0]
	// Interleave other evaluations, which would advance a shared RNG stream.
	simulation.Advance(from, 43, 1)
	simulation.Advance([][]float64{{99.0}}, 42, 3)
	again := simulation.Advance(from, 42, 1)[0][0]

	if first != again {
		t.Fatalf("not reproducible across interleaved runs: %v then %v", first, again)
	}
	if other := simulation.Advance(from, 43, 1)[0][0]; other == first {
		t.Fatalf("different seeds gave the same result (%v); the seed is not reaching "+
			"the iterations", other)
	}
}

func TestReentrantSimulationDoesNotRetainOrMutateInput(t *testing.T) {
	simulation := newTestReentrantSimulation(t)
	from := [][]float64{{5.0}}

	out := simulation.Advance(from, 7, 1)
	if from[0][0] != 5.0 {
		t.Fatalf("Advance mutated the rows it was given: %v", from)
	}
	// Mutating the returned rows must not affect a later identical call.
	out[0][0] = 12345
	if repeat := simulation.Advance(from, 7, 1)[0][0]; repeat == 12345 {
		t.Fatal("Advance returned a slice aliased to its own internal state")
	}
}

// TestReentrantSimulationStepsAreAuthoritative pins that the run length comes
// from the caller, not the sub-simulation's own termination condition (which is
// configured here for a single step).
func TestReentrantSimulationStepsAreAuthoritative(t *testing.T) {
	simulation := newTestReentrantSimulation(t)
	one := simulation.Advance([][]float64{{0}}, 5, 1)[0][0]
	many := simulation.Advance([][]float64{{0}}, 5, 20)[0][0]
	if one == many {
		t.Fatalf("1 step and 20 steps gave the same result (%v); steps is being ignored", one)
	}
	// With a positive drift the 20-step run must have travelled further.
	simulation.SetParam(0, "drift", []float64{10})
	drifted := simulation.Advance([][]float64{{0}}, 5, 20)[0][0]
	if drifted < 100 {
		t.Fatalf("20 steps of drift 10 should be far from the origin, got %v", drifted)
	}
}

func TestReentrantSimulationSetParamReachesTheIteration(t *testing.T) {
	simulation := newTestReentrantSimulation(t)
	base := simulation.Advance([][]float64{{0}}, 3, 1)[0][0]
	simulation.SetParam(0, "drift", []float64{100})
	shifted := simulation.Advance([][]float64{{0}}, 3, 1)[0][0]
	if shifted-base < 99 || shifted-base > 101 {
		t.Fatalf("drift param did not reach the iteration: %v then %v", base, shifted)
	}
}

func TestDeriveSeedSeparatesStreams(t *testing.T) {
	if simulator.DeriveSeed(7, 0) == simulator.DeriveSeed(7, 1) {
		t.Error("two partitions of the same run must not share a seed")
	}
	if simulator.DeriveSeed(7, 0) != simulator.DeriveSeed(7, 0) {
		t.Error("DeriveSeed must be deterministic")
	}
	if simulator.DeriveSeed(7, 0) == simulator.DeriveSeed(8, 0) {
		t.Error("different base seeds must give different partition seeds")
	}
}

func TestReentrantSimulationPartitionLookup(t *testing.T) {
	simulation := newTestReentrantSimulation(t)
	index, ok := simulation.PartitionIndex("walk")
	if !ok || index != 0 {
		t.Fatalf("PartitionIndex(walk) = %d, %v", index, ok)
	}
	if _, ok := simulation.PartitionIndex("absent"); ok {
		t.Error("PartitionIndex reported an unknown partition as present")
	}
	if widths := simulation.StateWidths(); len(widths) != 1 || widths[0] != 1 {
		t.Errorf("StateWidths = %v", widths)
	}
}
