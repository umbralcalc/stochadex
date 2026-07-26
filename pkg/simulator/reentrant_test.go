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

// deepCounterIteration increments a counter, so its history window holds a
// predictable descending sequence.
type deepCounterIteration struct{}

func (d *deepCounterIteration) Configure(int, *simulator.Settings) {}

func (d *deepCounterIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{stateHistories[partitionIndex].Values.RawRowView(0)[0] + 1}
}

func newDeepSimulation(t *testing.T, depth int) *simulator.ReentrantSimulation {
	t.Helper()
	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 4},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "counter",
		Iteration:         &deepCounterIteration{},
		InitStateValues:   []float64{0},
		StateHistoryDepth: depth,
		Seed:              1,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}
	return simulator.NewReentrantSimulation(settings, impl)
}

// TestReentrantSimulationWindowRoundTrip checks the pairing that lets a caller
// carry a model's whole history through its own state encoding: RunWindows out,
// NewStateHistoryFromWindow back in. Without it an iteration reading N steps back
// restarts from the initial value on every run.
func TestReentrantSimulationWindowRoundTrip(t *testing.T) {
	const depth = 4
	simulation := newDeepSimulation(t, depth)

	// One step from a window holding [3 2 1 0] must shift to [4 3 2 1].
	seed := uint64(11)
	windows := simulation.RunWindows(simulator.ReentrantRun{
		Histories: map[int]*simulator.StateHistory{
			0: simulator.NewStateHistoryFromWindow([]float64{3, 2, 1, 0}, 1, depth),
		},
		Seed:  &seed,
		Steps: 1,
	})
	if len(windows) != 1 || len(windows[0]) != depth {
		t.Fatalf("expected one %d-long window, got %v", depth, windows)
	}
	for row, want := range []float64{4, 3, 2, 1} {
		if windows[0][row] != want {
			t.Fatalf("window %v: row %d is %v, want %v (the window is not shifting)",
				windows[0], row, windows[0][row], want)
		}
	}

	// Feeding the result straight back must keep advancing, which is what makes
	// the round trip usable as a state encoding.
	next := simulation.RunWindows(simulator.ReentrantRun{
		Histories: map[int]*simulator.StateHistory{
			0: simulator.NewStateHistoryFromWindow(windows[0], 1, depth),
		},
		Seed:  &seed,
		Steps: 1,
	})
	if next[0][0] != 5 {
		t.Fatalf("second run gave %v, want the sequence to continue at 5", next[0])
	}
}

// TestNewStateHistoryFromWindowTolersShortInput checks a truncated window is
// filled as far as it goes rather than panicking, so a caller cannot crash the
// engine with a malformed encoding.
func TestNewStateHistoryFromWindowToleratesShortInput(t *testing.T) {
	history := simulator.NewStateHistoryFromWindow([]float64{7, 8}, 1, 4)
	if history.StateHistoryDepth != 4 || history.StateWidth != 1 {
		t.Fatalf("shape not preserved: depth %d width %d",
			history.StateHistoryDepth, history.StateWidth)
	}
	if history.Values.At(0, 0) != 7 || history.Values.At(1, 0) != 8 {
		t.Errorf("supplied rows not copied: %v %v",
			history.Values.At(0, 0), history.Values.At(1, 0))
	}
	if history.Values.At(3, 0) != 0 {
		t.Errorf("missing rows should stay zero, got %v", history.Values.At(3, 0))
	}
}

// TestReentrantSimulationRunIntoReusesBuffer pins the allocation-free path: the
// concatenation goes into the caller's slice, and its contents match Run's.
func TestReentrantSimulationRunIntoReusesBuffer(t *testing.T) {
	simulation := newTestReentrantSimulation(t)
	buffer := make([]float64, 0, 8)
	out := simulation.Advance([][]float64{{2}}, 5, 1)

	got := simulation.RunInto(
		simulator.ReentrantRun{Rows: [][]float64{{2}}, Seed: ptrUint64(5), Steps: 1}, buffer)
	if len(got) != 1 || got[0] != out[0][0] {
		t.Fatalf("RunInto gave %v, want the same as Run's %v", got, out)
	}
	if cap(got) != cap(buffer) {
		t.Errorf("RunInto allocated a new slice (cap %d, gave it %d)", cap(got), cap(buffer))
	}
}

// TestReentrantSimulationRunsToTerminationWithoutSteps covers Steps=0, where the
// sub-simulation's own termination condition decides the length.
func TestReentrantSimulationRunsToTerminationWithoutSteps(t *testing.T) {
	simulation := newDeepSimulation(t, 1)
	seed := uint64(3)
	rows := simulation.Run(simulator.ReentrantRun{
		Rows: [][]float64{{0}}, Seed: &seed, Steps: 0,
	})
	// The configured condition is four steps, so the counter must reach four.
	if rows[0][0] != 4 {
		t.Errorf("counter reached %v, want 4 from the configured termination condition",
			rows[0][0])
	}
}

func ptrUint64(v uint64) *uint64 { return &v }
