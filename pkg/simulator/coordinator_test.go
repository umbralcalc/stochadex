package simulator

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

// doublingProcessIteration defines an iteration which is only for
// testing - the process multiplies the values of the previous timestep
// by a factor of 2.
type doublingProcessIteration struct{}

func (d *doublingProcessIteration) Iterate(
	otherParams *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *TimestepsHistory,
) *State {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := 0; i < stateHistory.StateWidth; i++ {
		values[i] = stateHistory.Values.At(0, i) * 2.0
	}
	return &State{
		Values: mat.NewVecDense(
			stateHistory.StateWidth,
			values,
		),
		StateWidth: stateHistory.StateWidth,
	}
}

func iteratePartition(c *PartitionCoordinator, partitionIndex int) *State {
	// iterate this partition by one step within the same thread
	return c.iterators[partitionIndex].Iterate(
		c.stateHistories,
		c.timestepsHistory,
	)
}

func iterateHistory(c *PartitionCoordinator) {
	// update the state history for each job in turn within the same thread
	for index := 0; index < c.numberOfPartitions; index++ {
		state := iteratePartition(c, index)
		// reference this partition
		partition := c.stateHistories[index]
		// iterate over the history (matrix columns) and shift them
		// back one timestep
		for i := 1; i < partition.StateHistoryDepth; i++ {
			partition.Values.SetRow(i, partition.Values.RawRowView(i-1))
		}
		// update the latest state in the history
		partition.Values.SetRow(0, state.Values.RawVector().Data)
	}

	// iterate over the history of timesteps and shift them back one
	for i := 1; i < c.timestepsHistory.StateHistoryDepth; i++ {
		c.timestepsHistory.Values.SetVec(i, c.timestepsHistory.Values.AtVec(i-1))
	}
	// now update the history with the next time increment
	c.timestepsHistory.Values.SetVec(
		0,
		c.timestepsHistory.Values.AtVec(0)+c.timestepsHistory.NextIncrement,
	)
}

func run(c *PartitionCoordinator) {
	// terminate without iterating again if the condition has not been met
	for !c.terminationCondition.Terminate(
		c.stateHistories,
		c.timestepsHistory,
		c.overallTimesteps,
	) {
		c.overallTimesteps += 1
		c.timestepsHistory = c.timestepFunction.NextIncrement(c.timestepsHistory)
		iterateHistory(c)
	}
}

func TestPartitionCoordinator(t *testing.T) {
	t.Run(
		"test for the correct usage of goroutines in partition manager",
		func(t *testing.T) {
			params := &OtherParams{FloatParams: make(map[string][]float64)}
			settings := &LoadSettingsConfig{
				OtherParams:           []*OtherParams{params, params},
				InitStateValues:       [][]float64{{7.0, 8.0, 3.0}, {1.0, 2.0}},
				Seeds:                 []uint64{2365, 167},
				StateWidths:           []int{3, 2},
				StateHistoryDepths:    []int{2, 2},
				TimestepsHistoryDepth: 2,
			}
			iterations := make([]Iteration, 0)
			for range settings.StateWidths {
				iterations = append(iterations, &doublingProcessIteration{})
			}
			storeWithGoroutines := make([][][]float64, 2)
			implementations := &LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &VariableStoreOutputFunction{Store: storeWithGoroutines},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 10,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			configWithGoroutines := NewStochadexConfig(settings, implementations)
			coordWithGoroutines := NewPartitionCoordinator(configWithGoroutines)
			storeWithoutGoroutines := make([][][]float64, 2)
			outputWithoutGoroutines := &VariableStoreOutputFunction{
				Store: storeWithoutGoroutines,
			}
			implementations.OutputFunction = outputWithoutGoroutines
			configWithoutGoroutines := NewStochadexConfig(settings, implementations)
			coordWithoutGoroutines := NewPartitionCoordinator(configWithoutGoroutines)
			coordWithGoroutines.Run()
			run(coordWithoutGoroutines)
			for tIndex, store := range storeWithoutGoroutines {
				for pIndex, partitionStore := range store {
					for eIndex, element := range partitionStore {
						if element != storeWithGoroutines[tIndex][pIndex][eIndex] {
							t.Error("outputs with and without goroutines don't match")
						}
					}
				}
			}
		},
	)
}
