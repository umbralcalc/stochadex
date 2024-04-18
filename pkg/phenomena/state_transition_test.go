package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestStateTransitionIteration(t *testing.T) {
	t.Run(
		"test that the state transition iteration runs",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./state_transition_config.yaml")
			partitions := []simulator.Partition{
				{
					Iteration: &simulator.ConstantValuesIteration{},
				},
				{
					Iteration: &StateTransitionIteration{},
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
