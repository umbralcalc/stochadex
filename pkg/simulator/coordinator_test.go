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
	settings *LoadSettingsConfig,
) {
}

func (d *doublingProcessIteration) Iterate(
	otherParams *OtherParams,
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
	settings *LoadSettingsConfig,
) {
}

func (p *paramMultProcessIteration) Iterate(
	otherParams *OtherParams,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := 0; i < stateHistory.StateWidth; i++ {
		values[i] = stateHistory.Values.At(0, i) *
			otherParams.FloatParams["multipliers"][i]
	}
	return values
}

func iteratePartition(c *PartitionCoordinator, partitionIndex int) []float64 {
	// iterate this partition by one step within the same thread
	return c.Iterators[partitionIndex].Iterate(
		c.StateHistories,
		c.TimestepsHistory,
	)
}

func iterateHistory(c *PartitionCoordinator) {
	// update the state history for each job in turn within the same thread
	for index := 0; index < c.numberOfPartitions; index++ {
		state := iteratePartition(c, index)
		// reference this partition
		partition := c.StateHistories[index]
		// iterate over the history (matrix columns) and shift them
		// back one timestep
		for i := 1; i < partition.StateHistoryDepth; i++ {
			partition.Values.SetRow(i, partition.Values.RawRowView(i-1))
		}
		// update the latest state in the history
		partition.Values.SetRow(0, state)
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
	for !c.terminationCondition.Terminate(
		c.StateHistories,
		c.TimestepsHistory,
		c.overallTimesteps,
	) {
		c.overallTimesteps += 1
		c.TimestepsHistory = c.timestepFunction.SetNextIncrement(c.TimestepsHistory)
		iterateHistory(c)
	}
}

func TestPartitionCoordinator(t *testing.T) {
	t.Run(
		"test for the correct usage of goroutines in partition manager",
		func(t *testing.T) {
			params := make(map[string][]float64)
			params["multipliers"] = []float64{2.4, 1.0, 4.3, 3.2, 1.1}
			settings := &LoadSettingsConfig{
				OtherParams: []*OtherParams{
					{FloatParams: make(map[string][]float64)},
					{FloatParams: params},
				},
				InitStateValues: [][]float64{
					{7.0, 8.0, 3.0, 7.0, 1.0},
					{1.0, 2.0, 3.0},
				},
				Seeds:                 []uint64{2365, 167},
				StateWidths:           []int{5, 3},
				StateHistoryDepths:    []int{2, 10},
				TimestepsHistoryDepth: 10,
			}
			iterations := make([]Iteration, 0)
			iterations = append(iterations, &doublingProcessIteration{})
			iterations = append(iterations, &paramMultProcessIteration{})
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
