package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCopyValues(t *testing.T) {
	t.Run(
		"test that the copy values iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./copy_values_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationOne.Configure(0, settings)
			iterationTwo := &CopyValuesIteration{}
			iterationTwo.Configure(1, settings)
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationOne,
					iterationTwo,
				},
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
		"test that the copy values iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./copy_values_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationTwo := &CopyValuesIteration{}
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationOne,
					iterationTwo,
				},
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
