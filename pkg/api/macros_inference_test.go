package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

const posteriorMacroYAML = `data:
  steps: 500
  timestep: 1.0
  partitions:
  - name: test_data
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [1.8, 5.0], covariance_matrix: [2.5, 0.0, 0.0, 9.0]}
    init_state_values: [1.3, 8.3]
    state_history_depth: 500
    seed: 123
macros:
- type: posterior_estimation
  log_norm: {name: test_post_log_norm, default: 0.0}
  mean: {name: test_post_mean, default: [1.8, 5.0]}
  covariance: {name: test_post_cov, default: [2.5, 0.0, 0.0, 9.0]}
  sampler:
    name: test_post_sampler
    default: [1.8, 5.0]
    distribution:
      likelihood: {type: normal, allow_default_covariance_fallback: true}
      params: {default_covariance: [2.5, 0.0, 0.0, 9.0], cov_burn_in_steps: [200]}
      params_from_upstream:
        mean: {upstream: test_post_mean}
        covariance_matrix: {upstream: test_post_cov}
  comparison:
    name: test_likelihood
    model:
      likelihood: {type: normal}
      params: {mean: [1.8, 5.0], covariance_matrix: [2.5, 0.0, 0.0, 9.0]}
    data: {partition_name: test_data}
    window:
      data: [{partition_name: test_data}]
      depth: 200
    window_data_history_depth: {test_data: 200}
  past_discount: 1.0
  memory_depth: 200
  seed: 1234
`

// directPosteriorStorage builds the same scenario as posteriorMacroYAML with a
// hand-written Go AppliedPosteriorEstimation, so the macro can be checked against
// the constructor it wraps.
func directPosteriorStorage() *simulator.StateTimeStorage {
	storage := analysis.NewStateTimeStorageFromPartitions(
		[]*simulator.PartitionConfig{{
			Name:      "test_data",
			Iteration: &inference.DataGenerationIteration{Likelihood: &inference.NormalLikelihoodDistribution{}},
			Params: simulator.NewParams(map[string][]float64{
				"mean": {1.8, 5.0}, "covariance_matrix": {2.5, 0, 0, 9.0},
			}),
			InitStateValues: []float64{1.3, 8.3}, StateHistoryDepth: 500, Seed: 123,
		}},
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 500},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0}, 0.0,
	)
	partitions := analysis.NewPosteriorEstimationPartitions(
		analysis.AppliedPosteriorEstimation{
			LogNorm:    analysis.PosteriorLogNorm{Name: "test_post_log_norm", Default: 0.0},
			Mean:       analysis.PosteriorMean{Name: "test_post_mean", Default: []float64{1.8, 5.0}},
			Covariance: analysis.PosteriorCovariance{Name: "test_post_cov", Default: []float64{2.5, 0, 0, 9.0}},
			Sampler: analysis.PosteriorSampler{
				Name: "test_post_sampler", Default: []float64{1.8, 5.0},
				Distribution: analysis.ParameterisedModel{
					Likelihood: &inference.NormalLikelihoodDistribution{AllowDefaultCovarianceFallback: true},
					Params: simulator.NewParams(map[string][]float64{
						"default_covariance": {2.5, 0, 0, 9.0}, "cov_burn_in_steps": {200},
					}),
					ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
						"mean": {Upstream: "test_post_mean"}, "covariance_matrix": {Upstream: "test_post_cov"},
					},
				},
			},
			Comparison: analysis.AppliedLikelihoodComparison{
				Name: "test_likelihood",
				Model: analysis.ParameterisedModel{
					Likelihood: &inference.NormalLikelihoodDistribution{},
					Params: simulator.NewParams(map[string][]float64{
						"mean": {1.8, 5.0}, "covariance_matrix": {2.5, 0, 0, 9.0},
					}),
				},
				Data:                   analysis.DataRef{PartitionName: "test_data"},
				Window:                 analysis.WindowedPartitions{Data: []analysis.DataRef{{PartitionName: "test_data"}}, Depth: 200},
				WindowDataHistoryDepth: map[string]int{"test_data": 200},
			},
			PastDiscount: 1.0, MemoryDepth: 200, Seed: 1234,
		},
		storage,
	)
	return analysis.AddPartitionsToStateTimeStorage(storage, partitions, map[string]int{"test_data": 200})
}

// TestPosteriorEstimationMacroEquivalence proves the posterior_estimation macro
// produces output identical to a hand-written NewPosteriorEstimationPartitions
// call with the equivalent AppliedPosteriorEstimation — i.e. the spec->Applied
// translation is faithful.
func TestPosteriorEstimationMacroEquivalence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(posteriorMacroYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	macroStorage, err := runMacros(LoadApiRunConfigFromYaml(path))
	if err != nil {
		t.Fatal(err)
	}
	direct := directPosteriorStorage()

	for _, name := range []string{"test_post_mean", "test_post_cov", "test_post_log_norm"} {
		got := macroStorage.GetValues(name)
		want := direct.GetValues(name)
		if len(got) == 0 || len(want) == 0 {
			t.Fatalf("%s: missing output (got %d, want %d rows)", name, len(got), len(want))
		}
		gotFinal, wantFinal := got[len(got)-1], want[len(want)-1]
		for i := range wantFinal {
			if gotFinal[i] != wantFinal[i] {
				t.Errorf("%s[%d]: macro=%v direct=%v (translation not faithful)",
					name, i, gotFinal[i], wantFinal[i])
			}
		}
	}
}

// TestLikelihoodMeanFunctionFitMacroEquivalence proves the fit macro is a
// faithful wrapper of NewLikelihoodMeanFunctionFitPartition (same output as the
// hand-written Applied), independent of whether the chosen hyperparameters
// converge.
func TestLikelihoodMeanFunctionFitMacroEquivalence(t *testing.T) {
	const fitYAML = `data:
  steps: 100
  timestep: 1.0
  partitions:
  - name: test_data
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [2.0, 3.0], covariance_matrix: [1.0, 0.0, 0.0, 1.0]}
    init_state_values: [2.0, 3.0]
    state_history_depth: 100
    seed: 5
macros:
- type: likelihood_mean_function_fit
  name: mean_fit
  model:
    likelihood: {type: normal}
    params: {covariance_matrix: [1.0, 0.0, 0.0, 1.0]}
  gradient: {function: mean_gradient, width: 2}
  data: {partition_name: test_data}
  window: {depth: 10}
  learning_rate: 0.005
  descent_iterations: 3
  window_data_history_depth: {test_data: 10}
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(fitYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	macroStorage, err := runMacros(LoadApiRunConfigFromYaml(path))
	if err != nil {
		t.Fatal(err)
	}

	// Direct Go equivalent, same data and Applied.
	storage := analysis.NewStateTimeStorageFromPartitions(
		[]*simulator.PartitionConfig{{
			Name:      "test_data",
			Iteration: &inference.DataGenerationIteration{Likelihood: &inference.NormalLikelihoodDistribution{}},
			Params: simulator.NewParams(map[string][]float64{
				"mean": {2.0, 3.0}, "covariance_matrix": {1, 0, 0, 1},
			}),
			InitStateValues: []float64{2.0, 3.0}, StateHistoryDepth: 100, Seed: 5,
		}},
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0}, 0.0,
	)
	fit := analysis.NewLikelihoodMeanFunctionFitPartition(
		analysis.AppliedLikelihoodMeanFunctionFit{
			Name: "mean_fit",
			Model: analysis.ParameterisedModelWithGradient{
				Likelihood: &inference.NormalLikelihoodDistribution{},
				Params:     simulator.NewParams(map[string][]float64{"covariance_matrix": {1, 0, 0, 1}}),
			},
			Gradient:               analysis.LikelihoodMeanGradient{Function: inference.MeanGradientFunc, Width: 2},
			Data:                   analysis.DataRef{PartitionName: "test_data"},
			Window:                 analysis.WindowedPartitions{Depth: 10},
			LearningRate:           0.005,
			DescentIterations:      3,
			WindowDataHistoryDepth: map[string]int{"test_data": 10},
		},
		storage,
	)
	direct := analysis.AddPartitionsToStateTimeStorage(storage,
		[]*simulator.PartitionConfig{fit}, map[string]int{"test_data": 10})

	got := macroStorage.GetValues("mean_fit")
	want := direct.GetValues("mean_fit")
	if len(got) == 0 || len(want) == 0 {
		t.Fatalf("missing mean_fit output (got %d want %d)", len(got), len(want))
	}
	g, w := got[len(got)-1], want[len(want)-1]
	for i := range w {
		if g[i] != w[i] {
			t.Errorf("mean_fit[%d]: macro=%v direct=%v (translation not faithful)", i, g[i], w[i])
		}
	}
}
