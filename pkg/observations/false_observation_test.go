package observations

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestFalseStaticPartialStateObservationIteration(t *testing.T) {
	t.Run(
		"test that the false static partial state observation works",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"false_observation_config.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &simulator.ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &simulator.ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &FalseStaticPartialStateObservationIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"values_to_observe":       0,
						"false_observation_probs": 1,
					},
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
