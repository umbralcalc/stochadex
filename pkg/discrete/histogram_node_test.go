package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestHistogramNodeIteration(t *testing.T) {
	t.Run(
		"test that the histogram node iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./histogram_node_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration:                   &CategoricalStateTransitionIteration{},
					ParamsFromUpstreamPartition: map[string]int{"transition_rates": 0},
				},
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration:                   &CategoricalStateTransitionIteration{},
					ParamsFromUpstreamPartition: map[string]int{"transition_rates": 2},
				},
				{
					Iteration: &HistogramNodeIteration{},
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
				TimestepFunction: simulator.NewExponentialDistributionTimestepFunction(
					2.0, settings.Seeds[0],
				),
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
}
