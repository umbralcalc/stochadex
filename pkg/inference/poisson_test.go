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

func TestPoissonLogLikelihood(t *testing.T) {
	t.Run(
		"test that the Poisson log-likelihood runs",
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
		"test that the Poisson log-likelihood runs with harnesses",
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

func TestPoissonLogLikelihoodGradient(t *testing.T) {
	t.Run(
		"test that the Poisson log-likelihood gradient runs",
		func(t *testing.T) {
			dist := &PoissonLikelihoodDistribution{
				Src: rand.NewSource(123456),
			}
			mean := mat.NewVecDense(3, []float64{0.5, 1.0, 0.8})
			covariance := mat.NewSymDense(3, []float64{
				5.0, 0.0, 0.0, 0.0, 5.0, 0.0, 0.0, 0.0, 5.0,
			})
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples(mean, covariance)
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"poisson_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &PoissonLikelihoodDistribution{},
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
				OutputFunction:  &simulator.StdoutOutputFunction{},
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
		"test that the Poisson log-likelihood gradient runs with harnesses",
		func(t *testing.T) {
			dist := &PoissonLikelihoodDistribution{
				Src: rand.NewSource(123456),
			}
			mean := mat.NewVecDense(3, []float64{0.5, 1.0, 0.8})
			covariance := mat.NewSymDense(3, []float64{
				5.0, 0.0, 0.0, 0.0, 5.0, 0.0, 0.0, 0.0, 5.0,
			})
			batchData := make([]float64, 0)
			for range 100 {
				values := dist.GenerateNewSamples(mean, covariance)
				batchData = append(batchData, values...)
			}
			settings := simulator.LoadSettingsFromYaml(
				"poisson_gradient_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&DataComparisonGradientIteration{
					Likelihood:   &PoissonLikelihoodDistribution{},
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
