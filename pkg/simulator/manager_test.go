package simulator

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

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

type VariableStoreOutputFunction struct {
	Store [][][]float64
}

func (f *VariableStoreOutputFunction) Output(
	partitionIndex int,
	state *State,
	timesteps int,
) {
	f.Store[partitionIndex] = append(
		f.Store[partitionIndex],
		state.Values.RawVector().Data,
	)
}

func iteratePartition(m *PartitionManager, partitionIndex int) *State {
	// iterate this partition by one step within the same thread
	return m.iterators[partitionIndex].Iterate(
		m.stateHistories,
		m.timestepsHistory,
	)
}

func iterateHistory(m *PartitionManager) {
	// update the state history for each job in turn within the same thread
	for index := 0; index < m.numberOfPartitions; index++ {
		m.UpdateHistory(index, iteratePartition(m, index))
		m.partitionTimesteps[index] += 1
	}
	m.overallTimesteps += 1
	m.timestepsHistory = m.timestepFunction.Iterate(m.timestepsHistory)
}

func run(m *PartitionManager) {
	// terminate without iterating again if the condition has not been met
	for !m.terminationCondition.Terminate(
		m.stateHistories,
		m.timestepsHistory,
		m.overallTimesteps,
	) {
		iterateHistory(m)
	}
}

func TestPartitionManager(t *testing.T) {
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
			managerWithGoroutines := NewPartitionManager(configWithGoroutines)
			storeWithoutGoroutines := make([][][]float64, 2)
			outputWithoutGoroutines := &VariableStoreOutputFunction{
				Store: storeWithoutGoroutines,
			}
			implementations.OutputFunction = outputWithoutGoroutines
			configWithoutGoroutines := NewStochadexConfig(settings, implementations)
			managerWithoutGoroutines := NewPartitionManager(configWithoutGoroutines)
			managerWithGoroutines.Run()
			run(managerWithoutGoroutines)
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
