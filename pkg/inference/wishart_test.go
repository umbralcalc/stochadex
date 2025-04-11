package inference

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
)

func TestWishartLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Wishart log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"wishart_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &WishartLikelihoodDistribution{},
				},
				&DataComparisonIteration{
					Likelihood: &WishartLikelihoodDistribution{},
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
		"test that the Wishart log-likelihood runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"wishart_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &WishartLikelihoodDistribution{},
				},
				&DataComparisonIteration{
					Likelihood: &WishartLikelihoodDistribution{},
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

func TestWishartLogLikelihoodGradient(t *testing.T) {
	t.Run(
		"test that the Wishart log-likelihood gradient runs",
		func(t *testing.T) {
			dist := &WishartLikelihoodDistribution{
				Src: rand.NewSource(123456),
			}
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("degrees_of_freedom", []float64{45.0})
			params.Set("scale_matrix", []float64{
				7.0, 0.0, 0.0, 0.0, 2.7, 0.0, 0.0, 0.0, 1.8,
			})
			dist.SetParams(&params, 0, nil, nil)
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples()
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"wishart_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &WishartLikelihoodDistribution{},
					GradientFunc: MeanGradientFunc,
					Batch: &simulator.StateHistory{
						Values:            mat.NewDense(100, 9, batchData),
						StateWidth:        9,
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
		"test that the Wishart log-likelihood gradient runs with harnesses",
		func(t *testing.T) {
			dist := &WishartLikelihoodDistribution{
				Src: rand.NewSource(123456),
			}
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("degrees_of_freedom", []float64{45.0})
			params.Set("scale_matrix", []float64{
				7.0, 0.0, 0.0, 0.0, 2.7, 0.0, 0.0, 0.0, 1.8,
			})
			dist.SetParams(&params, 0, nil, nil)
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples()
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"wishart_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &WishartLikelihoodDistribution{},
					GradientFunc: MeanGradientFunc,
					Batch: &simulator.StateHistory{
						Values:            mat.NewDense(100, 9, batchData),
						StateWidth:        9,
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
