package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
)

func TestTDistributionLogLikelihood(t *testing.T) {
	t.Run(
		"test that the t-distribution log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"t_distribution_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &TLikelihoodDistribution{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&general.ValuesFunctionVectorCovarianceIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&DataComparisonIteration{
					Likelihood: &TLikelihoodDistribution{},
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
		"test that the t-distribution log-likelihood runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"t_distribution_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &TLikelihoodDistribution{},
				},
				&general.ValuesFunctionVectorMeanIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&general.ValuesFunctionVectorCovarianceIteration{
					Function: general.DataValuesFunction,
					Kernel:   &kernels.ExponentialIntegrationKernel{},
				},
				&DataComparisonIteration{
					Likelihood: &TLikelihoodDistribution{},
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

func TestTDistributionLogLikelihoodGradient(t *testing.T) {
	t.Run(
		"test that the t-distribution log-likelihood gradient runs",
		func(t *testing.T) {
			dist := &TLikelihoodDistribution{
				Src: rand.NewSource(123456),
			}
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("degrees_of_freedom", []float64{23.0})
			params.Set("mean", []float64{35.0, 3.6, 1.0})
			params.Set("covariance_matrix", []float64{
				1.0, 0.0, 0.0, 0.0, 3.0, 0.0, 0.0, 0.0, 2.0,
			})
			dist.SetParams(&params, 0, nil, nil)
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples()
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"t_distribution_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &TLikelihoodDistribution{},
					GradientFunc: MeanGradientFunc,
					Batch: &simulator.StateHistory{
						Values:            mat.NewDense(100, 3, batchData),
						StateWidth:        3,
						StateHistoryDepth: 100,
					},
				},
				&continuous.GradientDescentIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
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
		"test that the t-distribution log-likelihood gradient runs with harnesses",
		func(t *testing.T) {
			dist := &TLikelihoodDistribution{
				Src: rand.NewSource(123456),
			}
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("degrees_of_freedom", []float64{23.0})
			params.Set("mean", []float64{35.0, 3.6, 1.0})
			params.Set("covariance_matrix", []float64{
				1.0, 0.0, 0.0, 0.0, 3.0, 0.0, 0.0, 0.0, 2.0,
			})
			dist.SetParams(&params, 0, nil, nil)
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples()
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"t_distribution_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &TLikelihoodDistribution{},
					GradientFunc: MeanGradientFunc,
					Batch: &simulator.StateHistory{
						Values:            mat.NewDense(100, 3, batchData),
						StateWidth:        3,
						StateHistoryDepth: 100,
					},
				},
				&continuous.GradientDescentIteration{},
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
