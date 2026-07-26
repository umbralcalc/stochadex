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

// Settings exposes the underlying settings, for callers that need to reach
// starting conditions this type does not model — params keyed by name, or an
// iteration's own configuration. Mutating it changes what the next run does.
func (r *ReentrantSimulation) Settings() *Settings { return r.settings }

// Implementations exposes the underlying implementations, for callers that must
// reach the iteration objects themselves (injecting outer context, for example).
func (r *ReentrantSimulation) Implementations() *Implementations {
	return r.implementations
}

// ReentrantRun describes one evaluation of a sub-simulation: where it starts,
// how its randomness is fixed, and how long it goes on for.
type ReentrantRun struct {
	// Rows sets each partition's initial row, in partition order. A nil entry,
	// or a short slice, leaves that partition's configured values alone.
	Rows [][]float64
	// Histories seeds whole state-history windows for the named partition
	// indices, for models that read further back than one step. It takes
	// precedence over Rows for those partitions.
	Histories map[int]*StateHistory
	// InitTimeValue overrides the sub-simulation's start time when non-nil.
	InitTimeValue *float64
	// Seed, when non-nil, reseeds every iteration before the run so the result
	// depends only on this run's inputs. Nil leaves the iterations' random
	// streams where the last run left them — the streaming behaviour that
	// general.EmbeddedSimulationRunIteration has by default.
	Seed *uint64
	// Steps is how many steps to run. Zero defers to the sub-simulation's own
	// termination condition instead.
	Steps int
	// AfterConfigure runs once the iterations have been (re)configured and
	// before the run starts. Reseeding calls Configure, which the framework
	// requires to re-initialise all mutable state — so anything injected into an
	// iteration from outside (see general.StateMemoryIteration) has to be
	// re-applied here, or a reseeded run would lose it.
	AfterConfigure func()
}

// evaluate applies the run's starting conditions and drives the sub-simulation,
// returning the coordinator so callers can read the final rows in whatever shape
// they need.
func (r *ReentrantSimulation) evaluate(run ReentrantRun) *PartitionCoordinator {
	for index := range r.settings.Iterations {
		if index < len(run.Rows) && run.Rows[index] != nil {
			r.settings.Iterations[index].InitStateValues = append(
				[]float64(nil), run.Rows[index]...)
		}
	}
	if run.InitTimeValue != nil {
		r.settings.InitTimeValue = *run.InitTimeValue
	}
	if run.Seed != nil {
		ReseedIterations(r.settings, r.implementations, *run.Seed)
	}
	if run.AfterConfigure != nil {
		run.AfterConfigure()
	}

	coordinator := NewPartitionCoordinator(r.settings, r.implementations)
	for index, history := range run.Histories {
		coordinator.Shared.StateHistories[index] = history
	}
	if run.Steps > 0 {
		stepper := coordinator.NewStepper()
		for i := 0; i < run.Steps; i++ {
			stepper.Step()
		}
		stepper.Close()
	} else {
		coordinator.Run()
	}
	return coordinator
}

// Run evaluates the sub-simulation and returns the resulting rows, one per
// partition in partition order.
//
// With Seed set the run is pure with respect to its inputs: repeating a call
// repeats its result, whatever ran in between. Rows are copied in and out, so
// the caller's slices are never retained or mutated. Callers that only want the
// rows concatenated should prefer RunInto, which reuses a buffer instead of
// allocating one slice per partition.
func (r *ReentrantSimulation) Run(run ReentrantRun) [][]float64 {
	coordinator := r.evaluate(run)
	out := make([][]float64, len(r.widths))
	for index := range r.widths {
		out[index] = append(
			[]float64(nil), coordinator.Shared.StateHistories[index].Values.RawRowView(0)...)
	}
	return out
}

// RunInto is Run for callers that want every partition's final row concatenated
// into one slice they own. dst is truncated and reused, so a caller holding one
// buffer across runs pays no per-partition allocation — the shape an iteration
// wants, since its own return value is a single row.
//
// The returned slice aliases dst. Treat it as valid only until the next call.
func (r *ReentrantSimulation) RunInto(run ReentrantRun, dst []float64) []float64 {
	coordinator := r.evaluate(run)
	dst = dst[:0]
	for index := range r.widths {
		dst = append(dst, coordinator.Shared.StateHistories[index].Values.RawRowView(0)...)
	}
	return dst
}

// Advance is the pure, fixed-length form of Run: evaluate the sub-simulation for
// steps steps from rows, under seed. It is the shape a planner wants, where a
// transition has to be a function of (state, action).
func (r *ReentrantSimulation) Advance(rows [][]float64, seed uint64, steps int) [][]float64 {
	return r.Run(ReentrantRun{Rows: rows, Seed: &seed, Steps: steps})
}
