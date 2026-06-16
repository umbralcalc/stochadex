package simulator

import (
	"fmt"
	"sync"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

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
// returns the first harness error found. For the default execution (no
// strategy) it uses the manual Step loop so the per-step checks run between
// every step exactly as before; for an explicit strategy it delegates the loop
// to Run and checks the harnesses once it returns.
func runHarnessedToTermination(
	coordinator *PartitionCoordinator,
	harnesses []*IterationTestHarness,
) error {
	if coordinator.RunStrategy == nil {
		var wg sync.WaitGroup
		for !coordinator.ReadyToTerminate() {
			coordinator.Step(&wg)
			for _, harness := range harnesses {
				if harness.Err != nil {
					return harness.Err
				}
			}
		}
		return nil
	}
	coordinator.Run()
	for _, harness := range harnesses {
		if harness.Err != nil {
			return harness.Err
		}
	}
	return nil
}
