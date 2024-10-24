package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesFunctionWindowedWeightedCovarianceIteration(t *testing.T) {
	t.Run(
		"test that the values function windowed weighted covariance iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./values_function_windowed_weighted_covariance_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &ConstantValuesIteration{},
				},
				{
					Iteration: &ValuesFunctionWindowedWeightedMeanIteration{
						Function: DataValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values": {Upstream: 0},
					},
				},
				{
					Iteration: &ValuesFunctionWindowedWeightedCovarianceIteration{
						Function: DataValuesFunction,
						Kernel:   &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_data_values": {Upstream: 0},
						"mean":               {Upstream: 1},
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
