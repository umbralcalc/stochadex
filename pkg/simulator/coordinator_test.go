package simulator

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

// DoublingProcessIteration defines an iteration which is only for
// testing - the process multiplies the values of the previous timestep
// by a factor of 2.
type DoublingProcessIteration struct{}

func (d *DoublingProcessIteration) Iterate(
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
	c.overallTimesteps += 1
	c.timestepsHistory = c.timestepFunction.Iterate(c.timestepsHistory)
}

func run(c *PartitionCoordinator) {
	// terminate without iterating again if the condition has not been met
	for !c.terminationCondition.Terminate(
		c.stateHistories,
		c.timestepsHistory,
		c.overallTimesteps,
	) {
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
				iterations = append(iterations, &DoublingProcessIteration{})
			}
			storeWithGoroutines := make([][][]float64, 2)
			implementations := &LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &VariableStoreOutputFunction{Store: storeWithGoroutines},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 10,
				},
				TimestepFunction: &ConstantNoMemoryTimestepFunction{
					Stepsize: 1.0,
				},
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
