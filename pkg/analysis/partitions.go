package analysis

import (
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
