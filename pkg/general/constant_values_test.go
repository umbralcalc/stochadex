package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestConstantValues(t *testing.T) {
	t.Run(
		"test that the constant values iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./constant_values_settings.yaml")
			iteration := &ConstantValuesIteration{}
			iteration.Configure(0, settings)
			implementations := &simulator.Implementations{
				Iterations:      []simulator.Iteration{iteration},
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
	t.Run(
		"test that the constant values iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./constant_values_settings.yaml")
			iteration := &ConstantValuesIteration{}
			implementations := &simulator.Implementations{
				Iterations:      []simulator.Iteration{iteration},
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
