package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesGroupedAggregationIteration(t *testing.T) {
	t.Run(
		"test that the values grouped aggregation iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_grouped_aggregation_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationOne.Configure(0, settings)
			iterationTwo := &ConstantValuesIteration{}
			iterationTwo.Configure(1, settings)
			iterationThree := &ValuesGroupedAggregationIteration{
				Aggregation: CountAggregation,
				Kernel:      &kernels.InstantaneousIntegrationKernel{},
			}
			iterationThree.Configure(2, settings)
			iterationFour := &ValuesGroupedAggregationIteration{
				Aggregation: MeanAggregation,
				Kernel:      &kernels.InstantaneousIntegrationKernel{},
			}
			iterationFour.Configure(3, settings)
			partitions := []simulator.Partition{
				{Iteration: iterationOne},
				{Iteration: iterationTwo},
				{
					Iteration: iterationThree,
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_grouping_partition_0": {Upstream: 0},
						"latest_states_partition_1":   {Upstream: 1},
					},
				},
				{
					Iteration: iterationFour,
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"latest_grouping_partition_0": {Upstream: 0},
						"latest_states_partition_1":   {Upstream: 1},
					},
				},
			}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StdoutOutputFunction{},
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
