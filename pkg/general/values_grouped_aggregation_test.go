package general

import (
	"testing"

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
				ValuesFunction: PartitionRangesValuesFunction,
				AggFunction:    CountAggFunction,
			}
			iterationThree.Configure(2, settings)
			iterationFour := &ValuesGroupedAggregationIteration{
				ValuesFunction: PartitionRangesValuesFunction,
				AggFunction:    MeanAggFunction,
			}
			iterationFour.Configure(3, settings)
			partitions := []simulator.Partition{
				{Iteration: iterationOne},
				{Iteration: iterationTwo},
				{Iteration: iterationThree},
				{Iteration: iterationFour},
			}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.EveryStepOutputCondition{},
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
