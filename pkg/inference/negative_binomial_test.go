package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestNegativeBinomialLinkingLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Negative Binomial data linking log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"negative_binomial_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &NegativeBinomialLikelihoodDistribution{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesVarianceFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&DataComparisonIteration{
					Likelihood: &NegativeBinomialLikelihoodDistribution{},
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
		"test that the Negative Binomial data linking log-likelihood runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"negative_binomial_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &NegativeBinomialLikelihoodDistribution{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesVarianceFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&DataComparisonIteration{
					Likelihood: &NegativeBinomialLikelihoodDistribution{},
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
