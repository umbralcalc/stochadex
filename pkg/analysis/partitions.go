package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewStateTimeStorageFromPartitions generates a new simulator.StateTimeStorage
// by running a simulation with the specified partitions configured.
func NewStateTimeStorageFromPartitions(
	partitions []*simulator.PartitionConfig,
	termination simulator.TerminationCondition,
	timestep simulator.TimestepFunction,
	initTime float64,
) *simulator.StateTimeStorage {
	generator := simulator.NewConfigGenerator()
	storage := simulator.NewStateTimeStorage()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction: &simulator.StateTimeStorageOutputFunction{
			Store: storage,
		},
		TerminationCondition: termination,
		TimestepFunction:     timestep,
		InitTimeValue:        initTime,
	})
	for _, partition := range partitions {
		generator.SetPartition(partition)
	}
	coordinator := simulator.NewPartitionCoordinator(generator.GenerateConfigs())
	coordinator.Run()
	return storage
}

// AddPartitionToStateTimeStorage extends the state time storage with newly
// generated values from a specified partition.
func AddPartitionToStateTimeStorage(
	storage *simulator.StateTimeStorage,
	partition *simulator.PartitionConfig,
	windowSizeByPartition map[string]int,
) *simulator.StateTimeStorage {
	generator := simulator.NewConfigGenerator()
	times := storage.GetTimes()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.OnlyGivenPartitionsOutputCondition{
			Partitions: map[string]bool{partition.Name: true},
		},
		OutputFunction: &simulator.StateTimeStorageOutputFunction{
			Store: storage,
		},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: len(times) - 1,
		},
		TimestepFunction: &general.FromStorageTimestepFunction{
			Data: times,
		},
		InitTimeValue: times[0],
	})
	if windowSizeByPartition == nil {
		windowSizeByPartition = make(map[string]int)
	}
	for _, name := range storage.GetNames() {
		windowSize, ok := windowSizeByPartition[name]
		if !ok {
			windowSize = 1
		}
		data := storage.GetValues(name)
		generator.SetPartition(&simulator.PartitionConfig{
			Name: name,
			Iteration: &general.FromStorageIteration{
				Data: data,
			},
			Params:            simulator.NewParams(make(map[string][]float64)),
			InitStateValues:   data[0],
			StateHistoryDepth: windowSize,
			Seed:              0,
		})
	}
	generator.SetPartition(partition)
	coordinator := simulator.NewPartitionCoordinator(generator.GenerateConfigs())
	coordinator.Run()
	return storage
}
