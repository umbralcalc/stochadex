package simulator

// The re-entrant evaluation tier: running a simulation as a pure function of its
// inputs, rather than as a stream that carries state forward.
//
// PartitionCoordinator advances a simulation. RunSeededEnsemble runs several.
// RunWithHarnesses runs one under assertions. This file adds the fourth way to
// drive a simulation: evaluate it from an arbitrary starting state, under a
// chosen seed, and get the result back — with the guarantee that the same inputs
// always produce the same output.
//
// # Why this is not already possible
//
// An iteration's mutable state, its RNG included, lives in the iteration object
// and is established by Configure. Anything that builds a coordinator and steps
// it therefore inherits whatever RNG state the iterations happen to be in, so
// re-running a model "from the same place" silently continues the previous
// stream instead of repeating it. general.EmbeddedSimulationRunIteration works
// exactly this way: four calls with identical inputs give four different
// answers, which is correct for advancing a nested simulation once per outer
// step and wrong for anything that needs to evaluate the same model twice.
//
// # Why it works now
//
// The framework already requires that every iteration re-initialise all of its
// mutable state in Configure — the rule RunWithHarnesses enforces by running a
// simulation twice and comparing. That invariant is precisely what makes a pure
// re-evaluation possible: re-Configuring from a derived seed restores the
// iterations to a state that depends only on that seed. ReseedIterations is that
// operation, and ReentrantSimulation wraps it up with the run itself.
//
// Callers that need this: planning over a model (an MCTS transition must be pure,
// because the search materialises a successor once per edge and reuses it),
// particle propagation, and any re-evaluation of a windowed model that should be
// reproducible rather than dependent on how many times it has been called.

// DeriveSeed mixes a base seed with a stream index, so the partitions of one
// re-entrant run get distinct but jointly-determined seeds. Splitting this way
// means two runs sharing a base seed reproduce each other exactly, while two
// partitions within a run do not share a random stream.
func DeriveSeed(base uint64, stream int) uint64 {
	// Odd multiplier from the 64-bit golden ratio: cheap, and adequate for
	// decorrelating per-partition streams that are already PCG-seeded.
	return base ^ (uint64(stream+1) * 0x9e3779b97f4a7c15)
}

// ReseedIterations re-Configures every iteration with a seed derived from base,
// restoring them to a state that depends only on base. After this call the next
// run reproduces any earlier run made with the same base and starting state.
//
// This relies on the framework rule that Configure re-initialises all mutable
// state. An iteration that stashes state outside Configure's reach breaks the
// guarantee — which is the same defect RunWithHarnesses already fails on.
func ReseedIterations(settings *Settings, implementations *Implementations, base uint64) {
	for index := range settings.Iterations {
		settings.Iterations[index].Seed = DeriveSeed(base, index)
	}
	for index, iteration := range implementations.Iterations {
		iteration.Configure(index, settings)
	}
}

// ReentrantSimulation evaluates a sub-simulation as a pure function of a
// starting state, a seed, and a step count.
//
// It owns the settings and implementations it is given: Advance mutates their
// initial values, params and seeds in place. Do not share one across goroutines,
// and do not hand the same Implementations to a coordinator running concurrently.
type ReentrantSimulation struct {
	settings        *Settings
	implementations *Implementations
	widths          []int
	nameToIndex     map[string]int
}

// NewReentrantSimulation wraps a configured sub-simulation for re-entrant use.
//
// The caller usually wants implementations.ExecutionStrategy set to
// &InlineExecution{}: a re-entrant run is typically short and small, so the
// per-step goroutine round-trip of the default strategy dominates its cost.
func NewReentrantSimulation(
	settings *Settings,
	implementations *Implementations,
) *ReentrantSimulation {
	widths := make([]int, len(settings.Iterations))
	nameToIndex := make(map[string]int, len(settings.Iterations))
	for index, iteration := range settings.Iterations {
		widths[index] = iteration.StateWidth
		nameToIndex[iteration.Name] = index
	}
	return &ReentrantSimulation{
		settings:        settings,
		implementations: implementations,
		widths:          widths,
		nameToIndex:     nameToIndex,
	}
}

// PartitionIndex returns the index of a named partition, and whether it exists.
func (r *ReentrantSimulation) PartitionIndex(name string) (int, bool) {
	index, ok := r.nameToIndex[name]
	return index, ok
}

// StateWidths returns each partition's state width, in partition order.
func (r *ReentrantSimulation) StateWidths() []int { return r.widths }

// SetParam sets a params key on a partition by index, which is how an input that
// is not part of the state — a control, an action, a parameter draw — enters a
// re-entrant run.
func (r *ReentrantSimulation) SetParam(partition int, key string, values []float64) {
	r.settings.Iterations[partition].Params.Set(key, values)
}

// Advance runs the sub-simulation for the given number of steps from the given
// rows and returns the resulting rows, one per partition in partition order.
//
// Pure with respect to (rows, params, seed, steps): the iterations are reseeded
// from seed before the run, so repeating a call repeats its result.
//
// steps is authoritative — the sub-simulation's own termination condition is not
// consulted, because a re-entrant run's length is the caller's decision rather
// than the configuration's. Rows are copied in and out, so the caller's slices
// are never retained or mutated.
func (r *ReentrantSimulation) Advance(rows [][]float64, seed uint64, steps int) [][]float64 {
	for index := range r.settings.Iterations {
		if index < len(rows) && rows[index] != nil {
			r.settings.Iterations[index].InitStateValues = append(
				[]float64(nil), rows[index]...)
		}
	}
	ReseedIterations(r.settings, r.implementations, seed)

	coordinator := NewPartitionCoordinator(r.settings, r.implementations)
	stepper := coordinator.NewStepper()
	for i := 0; i < steps; i++ {
		stepper.Step()
	}
	stepper.Close()

	out := make([][]float64, len(r.widths))
	for index := range r.widths {
		out[index] = append(
			[]float64(nil), coordinator.Shared.StateHistories[index].Values.RawRowView(0)...)
	}
	return out
}
