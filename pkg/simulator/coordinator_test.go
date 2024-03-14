package simulator

import (
	"testing"
)

// doublingProcessIteration defines an iteration which is only for
// testing - the process multiplies the values of the previous timestep
// by a factor of 2.
type doublingProcessIteration struct{}

func (d *doublingProcessIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (d *doublingProcessIteration) Iterate(
	params *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := 0; i < stateHistory.StateWidth; i++ {
		values[i] = stateHistory.Values.At(0, i) * 2.0
	}
	return values
}

// paramMultProcessIteration defines an iteration which is only for
// testing - the process multiplies the values of the previous timestep
// by factors passed as a slice in otherParams.FloatParams["multipliers"].
type paramMultProcessIteration struct{}

func (p *paramMultProcessIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
}

func (p *paramMultProcessIteration) Iterate(
	params *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := 0; i < stateHistory.StateWidth; i++ {
		values[i] = stateHistory.Values.At(0, i) *
			params.FloatParams["multipliers"][i]
	}
	return values
}

func iteratePartition(
	c *PartitionCoordinator,
	partitionIndex int,
) []float64 {
	// iterate this partition by one step within the same thread
	return c.Iterators[partitionIndex].Iterate(
		c.StateHistories,
		c.TimestepsHistory,
	)
}

func iterateHistory(c *PartitionCoordinator) {
	// update the state history for each job in turn within the same thread
	for partitionIndex, stateHistory := range c.StateHistories {
		state := iteratePartition(c, partitionIndex)
		// iterate over the history (matrix columns) and shift them
		// back one timestep
		for i := 1; i < stateHistory.StateHistoryDepth; i++ {
			stateHistory.Values.SetRow(i, stateHistory.Values.RawRowView(i-1))
		}
		// update the latest state in the history
		stateHistory.Values.SetRow(0, state)
	}

	// iterate over the history of timesteps and shift them back one
	for i := 1; i < c.TimestepsHistory.StateHistoryDepth; i++ {
		c.TimestepsHistory.Values.SetVec(i, c.TimestepsHistory.Values.AtVec(i-1))
	}
	// now update the history with the next time increment
	c.TimestepsHistory.Values.SetVec(
		0,
		c.TimestepsHistory.Values.AtVec(0)+c.TimestepsHistory.NextIncrement,
	)
}

func run(c *PartitionCoordinator) {
	// terminate without iterating again if the condition has not been met
	for !c.TerminationCondition.Terminate(
		c.StateHistories,
		c.TimestepsHistory,
	) {
		c.TimestepsHistory.CurrentStepNumber += 1
		c.TimestepsHistory =
			c.TimestepFunction.SetNextIncrement(c.TimestepsHistory)
		iterateHistory(c)
	}
}

func TestPartitionCoordinator(t *testing.T) {
	t.Run(
		"test for the correct usage of goroutines in partition manager",
		func(t *testing.T) {
			params := make(map[string][]float64)
			params["multipliers"] = []float64{2.4, 1.0, 4.3, 3.2, 1.1}
			settings := &Settings{
				OtherParams: []*OtherParams{
					{FloatParams: make(map[string][]float64)},
					{FloatParams: params},
				},
				InitStateValues: [][]float64{
					{7.0, 8.0, 3.0, 7.0, 1.0},
					{1.0, 2.0, 3.0},
				},
				InitTimeValue:         0.0,
				Seeds:                 []uint64{2365, 167},
				StateWidths:           []int{5, 3},
				StateHistoryDepths:    []int{2, 10},
				TimestepsHistoryDepth: 10,
			}
			partitions := make([]Partition, 0)
			partitions = append(partitions, Partition{Iteration: &doublingProcessIteration{}})
			partitions = append(partitions, Partition{Iteration: &paramMultProcessIteration{}})
			for index, partition := range partitions {
				partition.Iteration.Configure(index, settings)
			}
			storeWithGoroutines := make([][][]float64, 2)
			implementations := &Implementations{
				Partitions:      partitions,
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &VariableStoreOutputFunction{Store: storeWithGoroutines},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 10,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordWithGoroutines := NewPartitionCoordinator(settings, implementations)
			storeWithoutGoroutines := make([][][]float64, 2)
			outputWithoutGoroutines := &VariableStoreOutputFunction{
				Store: storeWithoutGoroutines,
			}
			implementations.OutputFunction = outputWithoutGoroutines
			coordWithoutGoroutines := NewPartitionCoordinator(settings, implementations)
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
