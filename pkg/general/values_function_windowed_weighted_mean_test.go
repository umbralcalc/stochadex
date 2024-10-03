package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesFunctionWindowedWeightedMeanIteration(t *testing.T) {
	t.Run(
		"test that the values function windowed weighted mean iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_windowed_weighted_mean_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &ConstantValuesIteration{},
				},
				{
					Iteration: &ParamValuesIteration{},
				},
				{
					Iteration: &ValuesFunctionWindowedWeightedMeanIteration{
						Function: OtherValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"latest_data_values":  0,
						"latest_other_values": 1,
					},
				},
				{
					Iteration: &ValuesFunctionWindowedWeightedMeanIteration{
						Function: WeightedMeanValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"latest_data_values_partition_1": 1,
						"latest_data_values_partition_2": 2,
					},
				},
			}
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
