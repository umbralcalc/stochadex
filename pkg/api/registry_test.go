package api

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// wantIterationType maps each registered name to the concrete Go type it must
// construct. Drift test 1 asserts the registry builds exactly these types, so a
// name silently rebound to a different iteration fails CI.
var wantIterationType = map[string]string{
	"wiener_process":                      "*continuous.WienerProcessIteration",
	"gradient_descent":                    "*continuous.GradientDescentIteration",
	"ornstein_uhlenbeck":                  "*continuous.OrnsteinUhlenbeckIteration",
	"ornstein_uhlenbeck_exact_gaussian":   "*continuous.OrnsteinUhlenbeckExactGaussianIteration",
	"geometric_brownian_motion":           "*continuous.GeometricBrownianMotionIteration",
	"drift_diffusion":                     "*continuous.DriftDiffusionIteration",
	"cumulative_time":                     "*continuous.CumulativeTimeIteration",
	"poisson_process":                     "*discrete.PoissonProcessIteration",
	"cox_process":                         "*discrete.CoxProcessIteration",
	"bernoulli_process":                   "*discrete.BernoulliProcessIteration",
	"binomial_observation_process":        "*discrete.BinomialObservationProcessIteration",
	"categorical_state_transition":        "*discrete.CategoricalStateTransitionIteration",
	"hawkes_process":                      "*discrete.HawkesProcessIteration",
	"values_sorted_collection_mean":       "*general.ValuesSortedCollectionMeanIteration",
	"values_sorted_collection_covariance": "*general.ValuesSortedCollectionCovarianceIteration",
	"constant_values":                     "*general.ConstantValuesIteration",
	"copy_values":                         "*general.CopyValuesIteration",
	"param_values":                        "*general.ParamValuesIteration",
	"posterior_covariance":                "*inference.PosteriorCovarianceIteration",
	"posterior_log_normalisation":         "*inference.PosteriorLogNormalisationIteration",
	"smc_posterior":                       "*inference.SMCPosteriorIteration",
	"from_history":                        "*general.FromHistoryIteration",

	// composable (Phase B)
	"compound_poisson_process":          "*continuous.CompoundPoissonProcessIteration",
	"drift_jump_diffusion":              "*continuous.DriftJumpDiffusionIteration",
	"values_function_vector_mean":       "*general.ValuesFunctionVectorMeanIteration",
	"values_function_vector_covariance": "*general.ValuesFunctionVectorCovarianceIteration",
	"values_grouped_aggregation":        "*general.ValuesGroupedAggregationIteration",
	"cumulative":                        "*general.CumulativeIteration",
	"discounted_cumulative":             "*general.DiscountedCumulativeIteration",
	"values_function":                   "*general.ValuesFunctionIteration",
	"values_collection":                 "*general.ValuesCollectionIteration",
	"values_sorting_collection":         "*general.ValuesSortingCollectionIteration",
	"data_generation":                   "*inference.DataGenerationIteration",
	"data_comparison":                   "*inference.DataComparisonIteration",
	"expression":                        "*general.ExpressionIteration",
	"posterior_mean":                    "*inference.PosteriorMeanIteration",
	"smc_proposal":                      "*inference.SMCProposalIteration",
}

// iterationSpecFixtures gives a minimal valid Fields map for each composable
// iteration so drift test 1 can construct it. Data-only iterations are absent
// (they build from nil).
var iterationSpecFixtures = map[string]map[string]interface{}{
	"compound_poisson_process":          {"jump_dist": map[string]interface{}{"type": "gamma_jump"}},
	"drift_jump_diffusion":              {"jump_dist": map[string]interface{}{"type": "gamma_jump"}},
	"values_function_vector_mean":       {"function": "data_values", "kernel": map[string]interface{}{"type": "exponential"}},
	"values_function_vector_covariance": {"function": "data_values", "kernel": map[string]interface{}{"type": "exponential"}},
	"values_grouped_aggregation":        {"aggregation": "sum", "kernel": map[string]interface{}{"type": "exponential"}},
	"cumulative":                        {"iteration": map[string]interface{}{"type": "wiener_process"}},
	"discounted_cumulative":             {"iteration": map[string]interface{}{"type": "wiener_process"}},
	"values_function":                   {"transform": "params", "reduce": "sum"},
	"values_collection":                 {"pop_index": "next_non_empty", "push": "param_values"},
	"values_sorting_collection":         {"push_and_sort": "param_values"},
	"expression":                        {"fields": []interface{}{map[string]interface{}{"name": "x"}}, "outputs": []interface{}{"x"}},
	"data_generation":                   {"likelihood": map[string]interface{}{"type": "normal"}},
	"data_comparison":                   {"likelihood": map[string]interface{}{"type": "normal"}},
	"posterior_mean":                    {"transform": "mean"},
	"smc_proposal":                      {"priors": []interface{}{map[string]interface{}{"type": "uniform", "lo": 0.0, "hi": 1.0}}},
}

// TestIterationRegistryConstructs is drift test 1: every registered name builds a
// non-nil iteration of exactly the type it claims, and the type map and the
// registry name-sets agree (neither has an entry the other lacks).
func TestIterationRegistryConstructs(t *testing.T) {
	for name, build := range iterationBuilders {
		want, ok := wantIterationType[name]
		if !ok {
			t.Errorf("registered name %q has no expected-type entry in the test", name)
			continue
		}
		iteration, err := build(iterationSpecFixtures[name])
		if err != nil {
			t.Errorf("%q: build errored: %v", name, err)
			continue
		}
		if iteration == nil {
			t.Errorf("%q: build(nil) returned nil", name)
			continue
		}
		if got := reflect.TypeOf(iteration).String(); got != want {
			t.Errorf("%q: built %s, want %s", name, got, want)
		}
	}
	for name := range wantIterationType {
		if _, ok := iterationBuilders[name]; !ok {
			t.Errorf("expected-type map has %q but the registry does not", name)
		}
	}
}

func TestResolveIterationErrors(t *testing.T) {
	t.Run("an unknown type is rejected", func(t *testing.T) {
		if _, err := ResolveIteration(simulator.ComponentSpec{Type: "bogus_process"}); err == nil {
			t.Error("expected an error for an unknown iteration type")
		}
	})

	t.Run("a field on a no-field iteration is rejected", func(t *testing.T) {
		_, err := ResolveIteration(simulator.ComponentSpec{
			Type: "wiener_process", Fields: map[string]interface{}{"variances": []interface{}{1.0}},
		})
		if err == nil {
			t.Error("expected an error: params go in params:, not the iteration spec")
		}
	})

	t.Run("smc_posterior accepts param_names and rejects other fields", func(t *testing.T) {
		it, err := ResolveIteration(simulator.ComponentSpec{
			Type:   "smc_posterior",
			Fields: map[string]interface{}{"param_names": []interface{}{"a", "b"}},
		})
		if err != nil {
			t.Fatalf("param_names should be accepted: %v", err)
		}
		if got := it.(*inference.SMCPosteriorIteration).ParamNames; len(got) != 2 {
			t.Errorf("param_names not applied: %v", got)
		}
		if _, err := ResolveIteration(simulator.ComponentSpec{
			Type: "smc_posterior", Fields: map[string]interface{}{"nope": 1},
		}); err == nil {
			t.Error("expected an error for an unknown smc_posterior field")
		}
	})
}

// TestDataSpecIterationRunsInProcess is the integration test: a config using a
// framework iteration purely as data, with a data-spec simulation, is detected as
// fully data, resolves at load, and runs — no Go toolchain involved.
func TestDataSpecIterationRunsInProcess(t *testing.T) {
	const config = `main:
  partitions:
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0, 4.0]}
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 7
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if !LoadApiRunConfigStringsFromYaml(path).IsFullyData() {
		t.Fatal("a data-spec iteration + data-spec simulation should be fully data")
	}
	loaded := LoadApiRunConfigFromYaml(path)
	if loaded.Main.Partitions[0].Iteration == nil {
		t.Fatal("the data-spec iteration should be resolved at load")
	}
	if got := reflect.TypeOf(loaded.Main.Partitions[0].Iteration).String(); !strings.Contains(got, "WienerProcess") {
		t.Errorf("resolved iteration is %s, want a WienerProcessIteration", got)
	}
	// Run it: no panic, produces a generator that steps to termination.
	Run(loaded, &SocketConfig{})
}

// TestComposableResolution covers the recursive Phase B resolver: nested interface
// specs (kernel, likelihood, jump, prior), named funcs, and their error paths.
func TestComposableResolution(t *testing.T) {
	t.Run("a recursive product kernel resolves", func(t *testing.T) {
		it, err := ResolveIteration(simulator.ComponentSpec{
			Type: "values_function_vector_mean",
			Fields: map[string]interface{}{
				"function": "data_values",
				"kernel": map[string]interface{}{
					"type":     "product",
					"kernel_a": map[string]interface{}{"type": "exponential"},
					"kernel_b": map[string]interface{}{"type": "constant"},
				},
			},
		})
		if err != nil {
			t.Fatalf("product kernel should resolve: %v", err)
		}
		if it == nil {
			t.Fatal("nil iteration")
		}
	})

	t.Run("normal likelihood carries its bool field", func(t *testing.T) {
		it, err := ResolveIteration(simulator.ComponentSpec{
			Type: "data_generation",
			Fields: map[string]interface{}{
				"likelihood": map[string]interface{}{
					"type": "normal", "allow_default_covariance_fallback": true,
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		like := it.(*inference.DataGenerationIteration).Likelihood
		if !like.(*inference.NormalLikelihoodDistribution).AllowDefaultCovarianceFallback {
			t.Error("allow_default_covariance_fallback not applied through the nested spec")
		}
	})

	t.Run("smc_proposal resolves a list of priors", func(t *testing.T) {
		it, err := ResolveIteration(simulator.ComponentSpec{
			Type: "smc_proposal",
			Fields: map[string]interface{}{
				"priors": []interface{}{
					map[string]interface{}{"type": "uniform", "lo": 0.0, "hi": 1.0},
					map[string]interface{}{"type": "half_normal", "sigma": 2.0},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(it.(*inference.SMCProposalIteration).Priors); got != 2 {
			t.Errorf("resolved %d priors, want 2", got)
		}
	})

	t.Run("an unknown nested kernel type is rejected", func(t *testing.T) {
		_, err := ResolveIteration(simulator.ComponentSpec{
			Type: "values_function_vector_mean",
			Fields: map[string]interface{}{
				"function": "data_values",
				"kernel":   map[string]interface{}{"type": "bogus_kernel"},
			},
		})
		if err == nil {
			t.Error("expected an error for an unknown kernel type")
		}
	})

	t.Run("an unknown named function is rejected", func(t *testing.T) {
		_, err := ResolveIteration(simulator.ComponentSpec{
			Type: "values_function_vector_mean",
			Fields: map[string]interface{}{
				"function": "not_a_real_function",
				"kernel":   map[string]interface{}{"type": "exponential"},
			},
		})
		if err == nil {
			t.Error("expected an error for an unknown function name")
		}
	})

	t.Run("a missing required composable field is rejected", func(t *testing.T) {
		if _, err := ResolveIteration(simulator.ComponentSpec{
			Type: "data_generation", Fields: nil,
		}); err == nil {
			t.Error("expected an error for a missing likelihood")
		}
	})

	t.Run("an unknown field on a composable iteration is rejected", func(t *testing.T) {
		_, err := ResolveIteration(simulator.ComponentSpec{
			Type: "data_generation",
			Fields: map[string]interface{}{
				"likelihood": map[string]interface{}{"type": "normal"},
				"extra":      1,
			},
		})
		if err == nil {
			t.Error("expected an error for an unknown field")
		}
	})
}

// TestComposableRunsInProcess is the Phase B acceptance: a config whose iterations
// are composable data specs (data_generation with a nested normal likelihood
// feeding values_function_vector_mean with a named value function and an
// exponential kernel) is fully data, resolves recursively at load, and runs.
func TestComposableRunsInProcess(t *testing.T) {
	const config = `main:
  partitions:
  - name: data_stream
    iteration: {type: data_generation, likelihood: {type: normal, allow_default_covariance_fallback: true}}
    params:
      mean: [1.8, 5.0]
      covariance_matrix: [2.5, 0.0, 0.0, 9.0]
    init_state_values: [1.3, 8.3]
    state_history_depth: 50
    seed: 291
  - name: rolling_mean
    iteration: {type: values_function_vector_mean, function: data_values, kernel: {type: exponential}}
    params:
      exponential_weighting_timescale: [10.0]
    params_as_partitions:
      data_values_partition: [data_stream]
    params_from_upstream:
      latest_data_values: {upstream: data_stream}
    init_state_values: [1.8, 5.0]
    state_history_depth: 50
    seed: 0
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 100}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if !LoadApiRunConfigStringsFromYaml(path).IsFullyData() {
		t.Fatal("a composable-data config should be fully data")
	}
	loaded := LoadApiRunConfigFromYaml(path)
	for _, p := range loaded.Main.Partitions {
		if p.Iteration == nil {
			t.Fatalf("partition %q iteration not resolved at load", p.Name)
		}
	}
	Run(loaded, &SocketConfig{})
}

// TestFullInferenceConfigAsData is the Phase B acceptance test: the complete
// posterior-estimation inference model (cfg/example_inference_data_config.yaml),
// with embedded runs and from_history, is fully data, resolves at load, runs
// in-process, and — the point of doing inference at all — actually recovers the
// parameters that generated the data.
func TestFullInferenceConfigAsData(t *testing.T) {
	path := "../../cfg/example_inference_data_config.yaml"
	if !LoadApiRunConfigStringsFromYaml(path).IsFullyData() {
		t.Fatal("the data inference config should be detected as fully data")
	}
	config := LoadApiRunConfigFromYaml(path)
	// Run the fully-data config twice and capture params_posterior_mean each time.
	//
	// Determinism and finiteness are architecture-independent; a hard-coded exact
	// value is not, because this nonlinear Bayesian loop amplifies sub-ULP exp/log/sqrt
	// differences (last-bit between architectures) into trajectory divergence. But the
	// *converged* estimate is a stable fixed point, so the meaningful, portable property
	// is convergence to the data-generating mean — asserted below with a generous
	// tolerance. Byte-for-byte data-path == Go-path agreement holds per machine (shared
	// float) and is covered by the macro equivalence tests above.
	first := runInferenceData(t, config)
	second := runInferenceData(t, LoadApiRunConfigFromYaml(path))

	if len(first) != 4 {
		t.Fatalf("params_posterior_mean width = %d, want 4", len(first))
	}
	for i, v := range first {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
			t.Errorf("posterior mean[%d] = %v is not finite/bounded", i, v)
		}
		if v != second[i] {
			t.Errorf("posterior mean[%d] not deterministic: %v vs %v", i, v, second[i])
		}
	}

	// The data stream is generated with mean [1.8, 5, -7.3, 2.2]; the posterior over the
	// model's mean parameter must converge toward it. With the shipped tuning it lands
	// within ~0.3 (L2); the threshold is generous so "it converged" is guarded without
	// being brittle. This is what catches an under-tuned config that runs but does not
	// infer — e.g. the previous past_discount 0.5 / diag(1) settings sat at L2 ~3.8.
	target := []float64{1.8, 5.0, -7.3, 2.2}
	var l2 float64
	for i := range target {
		d := first[i] - target[i]
		l2 += d * d
	}
	if l2 = math.Sqrt(l2); l2 > 1.5 {
		t.Errorf("posterior did not converge to the data mean: got %v, want ~%v (L2 %.2f > 1.5)",
			first, target, l2)
	}
}

// runInferenceData runs the loaded inference config to a StateTimeStorage sink and
// returns the final params_posterior_mean row.
func runInferenceData(t *testing.T, config *ApiRunConfig) []float64 {
	t.Helper()
	storage := simulator.NewStateTimeStorage()
	config.Main.Simulation.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: storage}
	config.Main.Simulation.TerminationCondition = &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 2000}
	Run(config, &SocketConfig{})
	values := storage.GetValues("params_posterior_mean")
	if len(values) == 0 {
		t.Fatal("no params_posterior_mean output recorded")
	}
	return values[len(values)-1]
}
