package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPosteriorMeanIteration(t *testing.T) {
	t.Run(
		"test that the posterior mean iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"posterior_mean_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &NormalLikelihoodDistribution{},
				},
				&general.ConstantValuesIteration{},
				&DataGenerationIteration{
					Likelihood: &NormalLikelihoodDistribution{},
				},
				&general.ConstantValuesIteration{},
				&PosteriorLogNormalisationIteration{},
				&PosteriorMeanIteration{Transform: MeanTransform},
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
		"test that the posterior mean iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"posterior_mean_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &NormalLikelihoodDistribution{},
				},
				&general.ConstantValuesIteration{},
				&DataGenerationIteration{
					Likelihood: &NormalLikelihoodDistribution{},
				},
				&general.ConstantValuesIteration{},
				&PosteriorLogNormalisationIteration{},
				&PosteriorMeanIteration{Transform: MeanTransform},
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
