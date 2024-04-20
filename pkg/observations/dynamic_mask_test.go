package observations

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestDynamicMaskStateObservationIteration(t *testing.T) {
	t.Run(
		"test that the dynamic mask state observation works",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"dynamic_mask_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &phenomena.WienerProcessIteration{},
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
					Iteration: &DynamicMaskStateObservationIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"values_to_observe": 0,
						"mask_values":       1,
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
