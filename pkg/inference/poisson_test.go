package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPoissonLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Poisson data linking log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"poisson_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &PoissonLikelihoodDistribution{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&DataComparisonIteration{
					Likelihood: &PoissonLikelihoodDistribution{},
				},
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
		"test that the Poisson data linking log-likelihood runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"poisson_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &PoissonLikelihoodDistribution{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&DataComparisonIteration{
					Likelihood: &PoissonLikelihoodDistribution{},
				},
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
