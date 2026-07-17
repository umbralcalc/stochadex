package simulator

import (
	"fmt"
	"unsafe"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// outputAliasesHistory reports whether the slice returned by an iteration
// shares backing memory with any row of the partition's live state history.
//
// Returning such a slice is a contract violation: the engine and every output
// sink may retain the returned slice (copying it), but if it aliases live
// history then (a) mutating it corrupts the history in place — racily, since
// the iteration phase runs partitions concurrently — and (b) the aliased row
// keeps changing under anything that retained it. The depth>1 retention check
// catches in-place mutation of deeper rows; this catches the row-0 aliasing
// that is otherwise invisible when StateHistoryDepth == 1. Returning the
// dedicated NextValues buffer (e.g. via GetNextStateRowToUpdate) is fine — it
// is separately allocated and does not alias Values.
func outputAliasesHistory(output []float64, values *mat.Dense) bool {
	if len(output) == 0 {
		return false
	}
	data := values.RawMatrix().Data
	if len(data) == 0 {
		return false
	}
	outPtr := uintptr(unsafe.Pointer(&output[0]))
	base := uintptr(unsafe.Pointer(&data[0]))
	end := base + uintptr(len(data))*unsafe.Sizeof(data[0])
	return outPtr >= base && outPtr < end
}

// IterationTestHarness wraps an iteration and performs checks
// on its behaviour while running.
type IterationTestHarness struct {
	Iteration Iteration
	Err       error
	name      string
	history   *mat.Dense
}

func (h *IterationTestHarness) Configure(
	partitionIndex int,
	settings *Settings,
) {
	h.name = settings.Iterations[partitionIndex].Name
	h.history = mat.NewDense(
		settings.Iterations[partitionIndex].StateHistoryDepth,
		settings.Iterations[partitionIndex].StateWidth,
		nil,
	)
	h.history.SetRow(0, settings.Iterations[partitionIndex].InitStateValues)
	h.Iteration.Configure(partitionIndex, settings)
}

func (h *IterationTestHarness) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	paramsMapCopy := make(map[string][]float64)
	for paramsName, paramsValues := range params.Map {
		paramsValuesCopy := make([]float64, len(paramsValues))
		copy(paramsValuesCopy, paramsValues)
		paramsMapCopy[paramsName] = paramsValuesCopy
	}
	output := h.Iteration.Iterate(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	if len(params.Map) != len(paramsMapCopy) {
		h.Err = fmt.Errorf(
			"partition: %s, time: %f params values were mutated by iteration",
			h.name,
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
		)
		return output
	}
	for paramsName, paramsValues := range params.Map {
		for i, paramsValue := range paramsValues {
			if paramsMapCopy[paramsName][i] != paramsValue {
				h.Err = fmt.Errorf(
					"partition: %s, time: %f params values were mutated by iteration",
					h.name,
					timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
				)
				return output
			}
		}
	}
	if floats.HasNaN(output) {
		h.Err = fmt.Errorf("partition: %s, time: %f output state has NaN... %f",
			h.name,
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
			output)
		return output
	}
	stateHistory := stateHistories[partitionIndex]
	if len(output) != stateHistory.StateWidth {
		h.Err = fmt.Errorf("partition: %s, time: %f output state is wrong width..."+
			" %d should be: %d",
			h.name,
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
			len(output),
			stateHistory.StateWidth,
		)
		return output
	}
	if stateHistory.Values.RawMatrix().Rows != stateHistory.StateHistoryDepth {
		h.Err = fmt.Errorf("partition: %s, time: %f state history has wrong depth..."+
			" %d should be: %d",
			h.name,
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
			stateHistory.Values.RawMatrix().Rows,
			stateHistory.StateHistoryDepth)
		return output
	}
	if outputAliasesHistory(output, stateHistory.Values) {
		h.Err = fmt.Errorf(
			"partition: %s, time: %f output state aliases a row of live state"+
				" history; return a freshly allocated slice or the reusable"+
				" NextValues buffer (e.g. via GetNextStateRowToUpdate) instead",
			h.name,
			timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
		)
		return output
	}
	for i := stateHistory.StateHistoryDepth - 1; i > 0; i-- {
		pastState := h.history.RawRowView(i)
		pastStateFromHistory := stateHistory.Values.RawRowView(i)
		if !floats.Equal(pastStateFromHistory, pastState) {
			h.Err = fmt.Errorf(
				"partition: %s, time: %f, history depth: %d past state in history isn't"+
					" retained properly... %f should be: %f",
				h.name,
				timestepsHistory.Values.AtVec(0)+timestepsHistory.NextIncrement,
				i,
				pastStateFromHistory,
				pastState,
			)
			return output
		}
	}
	outputCopy := make([]float64, stateHistory.StateWidth)
	copy(outputCopy, output)
	for i := stateHistory.StateHistoryDepth - 1; i > 0; i-- {
		h.history.SetRow(i, h.history.RawRowView(i-1))
	}
	h.history.SetRow(0, outputCopy)
	return output
}

// checkStoredRowsDistinct verifies that the StateTimeStorage retained an
// independent copy of every appended state, rather than aliasing a reusable
// buffer that the producing iteration overwrites each step. It inspects the
// backing arrays of consecutive stored rows directly (the public GetValues
// deep-copies, which would mask the very aliasing under test). The realistic
// failure mode — an output sink that stops copying on retain — collapses an
// entire partition's trajectory onto one shared buffer, so detecting that any
// two consecutive rows share a base pointer is sufficient.
func checkStoredRowsDistinct(store *StateTimeStorage) error {
	for index, rows := range store.store {
		for i := 1; i < len(rows); i++ {
			if len(rows[i]) == 0 || len(rows[i-1]) == 0 {
				continue
			}
			if uintptr(unsafe.Pointer(&rows[i][0])) ==
				uintptr(unsafe.Pointer(&rows[i-1][0])) {
				return fmt.Errorf("stored output rows for partition index %d share"+
					" backing memory across timesteps; an output sink retained a"+
					" reusable buffer without copying on retain", index)
			}
		}
	}
	return nil
}

// RunWithHarnesses runs all iterations, each wrapped in a test harness and
// returns any errors if found. The simulation is also run twice to check
// for statefulness residues.
//
// It uses the default spawn-per-step execution. To exercise a specific
// ExecutionStrategy under the same checks, use RunWithHarnessesUsing.
func RunWithHarnesses(settings *Settings, implementations *Implementations) error {
	return RunWithHarnessesUsing(settings, implementations, nil)
}

// RunWithHarnessesUsing is like RunWithHarnesses but advances the simulation
// with the given ExecutionStrategy (nil selects the default spawn-per-step
// execution and uses the manual Step loop so behaviour is unchanged). It
// applies every per-step correctness check (params mutation, NaN, state width,
// history integrity) and the twice-run statefulness-residue check, so each
// strategy is validated against the same rigour as the default.
func RunWithHarnessesUsing(
	settings *Settings,
	implementations *Implementations,
	strategy ExecutionStrategy,
) error {
	implementations.ExecutionStrategy = strategy
	initRunStore := NewStateTimeStorage()
	implementations.OutputFunction = &StateTimeStorageOutputFunction{
		Store: initRunStore,
	}
	harnesses := make([]*IterationTestHarness, 0)
	for index, iteration := range implementations.Iterations {
		harness := &IterationTestHarness{
			Err:       nil,
			Iteration: iteration,
		}
		harness.Configure(index, settings)
		harnesses = append(harnesses, harness)
		implementations.Iterations[index] = harness
	}
	coordinator := NewPartitionCoordinator(settings, implementations)
	if err := runHarnessedToTermination(coordinator, harnesses); err != nil {
		return err
	}
	if err := checkStoredRowsDistinct(initRunStore); err != nil {
		return err
	}
	resetRunStore := NewStateTimeStorage()
	implementations.OutputFunction = &StateTimeStorageOutputFunction{
		Store: resetRunStore,
	}
	for index, iteration := range implementations.Iterations {
		iteration.Configure(index, settings)
	}
	coordinator = NewPartitionCoordinator(settings, implementations)
	if err := runHarnessedToTermination(coordinator, harnesses); err != nil {
		return err
	}
	for _, pName := range initRunStore.GetNames() {
		valuesAfterReset := resetRunStore.GetValues(pName)
		for tIndex, state := range initRunStore.GetValues(pName) {
			for valueIndex, value := range state {
				if value != valuesAfterReset[tIndex][valueIndex] {
					return fmt.Errorf("outputs pre- and post-reset don't match..." +
						" this typically happens if there is a statefulness residue")
				}
			}
		}
	}
	return nil
}

// runHarnessedToTermination advances the coordinator to termination and
// returns the first harness error found. It drives the coordinator through the
// configured strategy's Stepper, checking the harnesses between every step, so
// the per-step correctness checks run at the same granularity under every
// strategy — not just the default. Between two Steps the strategy has committed
// a full step and holds no work in flight, so inspecting the harnesses there is
// safe for the concurrent strategies too.
func runHarnessedToTermination(
	coordinator *PartitionCoordinator,
	harnesses []*IterationTestHarness,
) error {
	stepper := coordinator.NewStepper()
	defer stepper.Close()
	for !coordinator.ReadyToTerminate() {
		stepper.Step()
		for _, harness := range harnesses {
			if harness.Err != nil {
				return harness.Err
			}
		}
	}
	return nil
}
