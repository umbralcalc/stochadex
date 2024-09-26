package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCategoricalStateTransitionIteration(t *testing.T) {
	t.Run(
		"test that the state transition iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./categorical_state_transition_settings.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &general.ConstantValuesIteration{},
				},
				{
					Iteration: &CategoricalStateTransitionIteration{},
					ParamsFromUpstreamPartition: map[string]int{
						"transition_rates": 0,
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
