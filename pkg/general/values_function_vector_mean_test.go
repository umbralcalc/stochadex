package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesFunctionVectorMeanIteration(t *testing.T) {
	t.Run(
		"test that the values function vector mean iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_vector_mean_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &ConstantValuesIteration{},
				},
				{
					Iteration: &ParamValuesIteration{},
				},
				{
					Iteration: &ValuesFunctionVectorMeanIteration{
						Function: OtherValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values":  {Upstream: 0},
						"latest_other_values": {Upstream: 1},
					},
				},
				{
					Iteration: &ValuesFunctionVectorMeanIteration{
						Function: WeightedMeanValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values":             {Upstream: 0},
						"latest_data_values_partition_1": {Upstream: 1},
						"latest_data_values_partition_2": {Upstream: 2},
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
