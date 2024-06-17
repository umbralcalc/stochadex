package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestWeightedWindowedFunctionIteration(t *testing.T) {
	t.Run(
		"test that the weighted windowed function iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./weighted_windowed_function_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &WienerProcessIteration{},
				},
				{
					Iteration: &WienerProcessIteration{},
				},
				{
					Iteration: &WeightedWindowedFunctionIteration{
						Kernel: &kernels.ExponentialIntegrationKernel{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"latest_data_values":     0,
						"latest_function_values": 1,
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
