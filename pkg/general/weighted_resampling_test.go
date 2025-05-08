package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestWeightedResamplingIteration(t *testing.T) {
	t.Run(
		"test that the weighted resampling iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"weighted_resampling_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&WeightedResamplingIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
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
		"test that the weighted resampling iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"weighted_resampling_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&WeightedResamplingIteration{},
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
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
