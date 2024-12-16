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

			iterations := []simulator.Iteration{
				&general.ConstantValuesIteration{},
				&CategoricalStateTransitionIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: simulator.NewExponentialDistributionTimestepFunction(
					2.0, settings.Iterations[0].Seed,
				),
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
	t.Run(
		"test that the state transition iteration runs with harnesses",
		func(t *testing.T) {
			settings :=
				simulator.LoadSettingsFromYaml("./categorical_state_transition_settings.yaml")

			iterations := []simulator.Iteration{
				&general.ConstantValuesIteration{},
				&CategoricalStateTransitionIteration{},
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: simulator.NewExponentialDistributionTimestepFunction(
					2.0, settings.Iterations[0].Seed,
				),
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
