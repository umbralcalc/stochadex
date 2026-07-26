package simulator

import "gonum.org/v1/gonum/mat"

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

// RunWindows is Run for callers that need each partition's whole state-history
// window back rather than just its latest row — models whose iterations read
// further back than one step, where the window IS part of the state.
//
// Each returned slice is one partition's window flattened row-major with the
// latest row first, so it round-trips through NewStateHistoryFromWindow.
func (r *ReentrantSimulation) RunWindows(run ReentrantRun) [][]float64 {
	coordinator := r.evaluate(run)
	out := make([][]float64, len(r.widths))
	for index := range r.widths {
		history := coordinator.Shared.StateHistories[index]
		window := make([]float64, 0, history.StateHistoryDepth*history.StateWidth)
		for row := 0; row < history.StateHistoryDepth; row++ {
			window = append(window, history.Values.RawRowView(row)...)
		}
		out[index] = window
	}
	return out
}

// NewStateHistoryFromWindow builds a StateHistory from a flattened window,
// row-major with the latest row first — the shape ReentrantSimulation.RunWindows
// returns, so a caller can carry a model's whole window through its own state
// encoding and hand it back to start the next run.
func NewStateHistoryFromWindow(window []float64, width, depth int) *StateHistory {
	values := mat.NewDense(depth, width, nil)
	for row := 0; row < depth; row++ {
		offset := row * width
		if offset+width > len(window) {
			break
		}
		values.SetRow(row, window[offset:offset+width])
	}
	return &StateHistory{
		Values:            values,
		NextValues:        make([]float64, width),
		StateWidth:        width,
		StateHistoryDepth: depth,
	}
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
