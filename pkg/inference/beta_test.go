package inference

import (
	"testing"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

func TestBetaLogLikelihood(t *testing.T) {
	t.Run(
		"test that the beta log-likelihood runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"beta_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &BetaLikelihoodDistribution{},
				},
				&DataComparisonIteration{
					Likelihood: &BetaLikelihoodDistribution{},
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
		"test that the beta log-likelihood runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"beta_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataGenerationIteration{
					Likelihood: &BetaLikelihoodDistribution{},
				},
				&DataComparisonIteration{
					Likelihood: &BetaLikelihoodDistribution{},
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

func TestBetaLogLikelihoodGradient(t *testing.T) {
	t.Run(
		"test that the beta log-likelihood gradient runs",
		func(t *testing.T) {
			dist := &BetaLikelihoodDistribution{
				Src: rand.NewPCG(123456, 123456),
			}
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("alpha", []float64{
				1.0, 2.0, 3.0, 1.0, 2.0, 3.0, 1.0, 2.0, 3.0})
			params.Set("beta", []float64{
				2.0, 3.0, 1.0, 3.0, 2.0, 3.0, 1.0, 1.0, 1.0})
			dist.SetParams(&params, 0, nil, nil)
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples()
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"beta_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &BetaLikelihoodDistribution{},
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
		"test that the beta log-likelihood gradient runs with harnesses",
		func(t *testing.T) {
			dist := &BetaLikelihoodDistribution{
				Src: rand.NewPCG(123456, 123456),
			}
			params := simulator.NewParams(make(map[string][]float64))
			params.Set("alpha", []float64{
				1.0, 2.0, 3.0, 1.0, 2.0, 3.0, 1.0, 2.0, 3.0})
			params.Set("beta", []float64{
				2.0, 3.0, 1.0, 3.0, 2.0, 3.0, 1.0, 1.0, 1.0})
			dist.SetParams(&params, 0, nil, nil)
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples()
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"beta_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &BetaLikelihoodDistribution{},
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
