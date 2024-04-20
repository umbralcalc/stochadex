package simulator

import (
	"testing"
)

func TestEmbeddedSimulationRunIteration(t *testing.T) {
	t.Run(
		"test that the embedded simulation run iteration runs",
		func(t *testing.T) {
			embeddedSimPartitions := make([]Partition, 0)
			embeddedSimPartitions = append(
				embeddedSimPartitions,
				Partition{
					Iteration: &ConstantValuesIteration{},
				},
			)
			embeddedSimPartitions = append(
				embeddedSimPartitions,
				Partition{
					Iteration: &ConstantValuesIteration{},
				},
			)
			embeddedSettings := LoadSettingsFromYaml(
				"embedded_settings.yaml",
			)
			partitions := make([]Partition, 0)
			partitions = append(
				partitions,
				Partition{
					Iteration: &ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				Partition{
					Iteration: &ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				Partition{
					Iteration: NewEmbeddedSimulationRunIteration(
						LoadSettingsFromYaml("test_settings.yaml"),
						&Implementations{
							Partitions:      embeddedSimPartitions,
							OutputCondition: &NilOutputCondition{},
							OutputFunction:  &NilOutputFunction{},
							TerminationCondition: &NumberOfStepsTerminationCondition{
								MaxNumberOfSteps: 100,
							},
							TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
						},
					),
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, embeddedSettings)
			}
			implementations := &Implementations{
				Partitions:      partitions,
				OutputCondition: &NilOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := NewPartitionCoordinator(
				embeddedSettings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
