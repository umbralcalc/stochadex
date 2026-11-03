package inference

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestPosteriorCovarianceIteration(t *testing.T) {
	t.Run(
		"test that the posterior covariance iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"posterior_covariance_settings.yaml",
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
				&PosteriorCovarianceIteration{},
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
		"test that the posterior covariance iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"posterior_covariance_settings.yaml",
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
				&PosteriorCovarianceIteration{},
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
	t.Run(
		"test that the posterior covariance is the full weighted covariance matrix",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"posterior_covariance_settings.yaml",
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
				&PosteriorCovarianceIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			simulator.NewPartitionCoordinator(settings, &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 50,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}).Run()
			paramRows := [][][]float64{
				store.GetValues("partition_0"), store.GetValues("partition_2")}
			loglikeRows := [][][]float64{
				store.GetValues("partition_1"), store.GetValues("partition_3")}
			logNorms := store.GetValues("partition_4")
			means := store.GetValues("partition_5")
			got := store.GetValues("partition_6")
			// Recompute every step's matrix from the partitions it reads: the
			// latest normalisation and mean (wired within the step), and the
			// previous step's params and log-likelihoods (read from history),
			// accumulating every entry of the matrix from its initial state.
			const d = 3
			want := append([]float64(nil), settings.Iterations[6].InitStateValues...)
			for step := 1; step < len(got); step++ {
				c := logNorms[step][0]
				logTerms := []float64{c}
				for j := range loglikeRows {
					logTerms = append(logTerms, loglikeRows[j][step-1][0])
				}
				peak := floats.Max(logTerms)
				total := 0.0
				for _, l := range logTerms {
					total += math.Exp(l - peak)
				}
				logTotal := peak + math.Log(total)
				next := make([]float64, d*d)
				for i := range d * d {
					next[i] = math.Exp(c-logTotal) * want[i]
				}
				for j := range paramRows {
					w := math.Exp(loglikeRows[j][step-1][0] - logTotal)
					for r := range d {
						for k := range d {
							next[r*d+k] += w *
								(means[step][r] - paramRows[j][step-1][r]) *
								(means[step][k] - paramRows[j][step-1][k])
						}
					}
				}
				want = next
				if !floats.EqualApprox(got[step], want, 1e-9) {
					t.Fatalf("step %d: covariance %v, want %v", step, got[step], want)
				}
			}
			if final := got[len(got)-1]; final[1] == 0 {
				t.Errorf("final covariance %v should have off-diagonal entries", final)
			}
		},
	)
}
