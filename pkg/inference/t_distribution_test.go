package inference

import (
	"fmt"
	"strings"
	"testing"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
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
				Src: rand.NewPCG(123456, 123456),
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
				Src: rand.NewPCG(123456, 123456),
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

func TestTDistributionDefaultCovariance(t *testing.T) {
	t.Run(
		"test that a non-positive-definite covariance falls back to default_covariance",
		func(t *testing.T) {
			mean := []float64{0.5, -1.0}
			defaultCov := []float64{2.0, 0.3, 0.3, 1.5}
			params := simulator.NewParams(map[string][]float64{
				"degrees_of_freedom": {4.0},
				"mean":               mean,
				"covariance_matrix":  {1.0, 2.0, 2.0, 1.0},
				"default_covariance": defaultCov,
			})
			dist := &TLikelihoodDistribution{Src: rand.NewPCG(1, 1)}
			dist.SetParams(&params, 0, nil, nil)
			want, ok := distmv.NewStudentsT(
				mean, mat.NewSymDense(2, defaultCov), 4.0, nil)
			if !ok {
				t.Fatal("reference scale matrix is not positive-definite")
			}
			data := []float64{1.2, 0.4}
			if got := dist.EvaluateLogLike(data); got != want.LogProb(data) {
				t.Errorf("log-likelihood %v, want %v", got, want.LogProb(data))
			}
		},
	)
	t.Run(
		"test that default_covariance does not carry over once absent from params",
		func(t *testing.T) {
			withDefault := simulator.NewParams(map[string][]float64{
				"degrees_of_freedom": {4.0},
				"mean":               {0.0, 0.0},
				"covariance_matrix":  {1.0, 2.0, 2.0, 1.0},
				"default_covariance": {1.0, 0.0, 0.0, 1.0},
			})
			withoutDefault := simulator.NewParams(map[string][]float64{
				"degrees_of_freedom": {4.0},
				"mean":               {0.0, 0.0},
				"covariance_matrix":  {1.0, 2.0, 2.0, 1.0},
			})
			dist := &TLikelihoodDistribution{Src: rand.NewPCG(1, 1)}
			dist.SetParams(&withDefault, 0, nil, nil)
			dist.EvaluateLogLike([]float64{0.0, 0.0})
			dist.SetParams(&withoutDefault, 0, nil, nil)
			got := recoveredPanic(func() { dist.EvaluateLogLike([]float64{0.0, 0.0}) })
			if got != "covariance matrix is not positive-definite" {
				t.Errorf("panic %q, want the covariance reported as not positive-definite", got)
			}
		},
	)
	t.Run(
		"test that a non-positive-definite default_covariance panics clearly",
		func(t *testing.T) {
			params := simulator.NewParams(map[string][]float64{
				"degrees_of_freedom": {4.0},
				"mean":               {0.0, 0.0},
				"covariance_matrix":  {1.0, 2.0, 2.0, 1.0},
				"default_covariance": {1.0, 3.0, 3.0, 1.0},
			})
			dist := &TLikelihoodDistribution{Src: rand.NewPCG(1, 1)}
			dist.SetParams(&params, 0, nil, nil)
			for name, call := range map[string]func(){
				"EvaluateLogLike":    func() { dist.EvaluateLogLike([]float64{0.0, 0.0}) },
				"GenerateNewSamples": func() { dist.GenerateNewSamples() },
			} {
				got := recoveredPanic(call)
				if !strings.Contains(got, "default_covariance is also not positive-definite") {
					t.Errorf("%s panic %q, want default_covariance reported as not positive-definite",
						name, got)
				}
			}
		},
	)
}

// recoveredPanic runs call and returns the message it panicked with, or the
// empty string if it returned normally.
func recoveredPanic(call func()) (message string) {
	defer func() {
		if r := recover(); r != nil {
			message = fmt.Sprint(r)
		}
	}()
	call()
	return ""
}
