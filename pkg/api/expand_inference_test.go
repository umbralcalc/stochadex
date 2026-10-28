package api

import (
	"strings"
	"testing"
)

// inferenceInputs is a one-wide normal stream and a slow level beside it.
const inferenceInputs = `inputs:
  data:
    simulation:
      steps: 120
      timestep: 1.0
      partitions:
      - name: stream
        iteration: {type: data_generation, likelihood: {type: normal}}
        params: {mean: [1.5], covariance_matrix: [0.8]}
        init_state_values: [1.2]
        state_history_depth: 1
        seed: 17
      - name: pair
        iteration: {type: data_generation, likelihood: {type: normal}}
        params: {mean: [0.5, 2.0], covariance_matrix: [1.0, 0.4, 0.4, 2.0]}
        init_state_values: [0.4, 2.2]
        state_history_depth: 1
        seed: 29
      - name: flat
        iteration: {type: constant_values}
        params: {}
        init_state_values: [3.0]
        state_history_depth: 1
        seed: 0
      - name: level
        iteration: {type: wiener_process}
        params: {variances: [0.05]}
        init_state_values: [1.0]
        state_history_depth: 1
        seed: 5
macros:
`

// comparisonMacro is a standalone likelihood comparison whose window steps an
// Ornstein-Uhlenbeck process reverting to the outer level, scored against the
// stream.
const comparisonMacro = `- type: likelihood_comparison
  name: fit
  model:
    likelihood: {type: normal}
    params: {covariance_matrix: [0.8]}
    params_from_upstream:
      mean: {upstream: sim}
  data: {partition_name: stream}
  window:
    partitions:
    - partition:
        name: sim
        iteration: {type: ornstein_uhlenbeck}
        params: {thetas: [0.5], mus: [0.0], sigmas: [0.2]}
        init_state_values: [1.0]
        state_history_depth: 1
        seed: 8
      outside_upstreams:
        mus: {upstream: level}
    data: [{partition_name: stream}]
    depth: 10
  embedded_burn_in_steps: 4
  window_data_history_depth: {stream: 10}
`

// posteriorMacro estimates the OU process's reversion level, which a window
// partition takes from the sampler as an outside upstream, with per-dimension
// variances.
const posteriorMacro = `- type: posterior_estimation
  log_norm: {name: post_log_norm, default: 0.0}
  mean: {name: post_mean, default: [0.0]}
  covariance: {name: post_var, default: [4.0], just_variance: true}
  sampler:
    name: post_sampler
    default: [0.0]
    distribution:
      likelihood: {type: normal, allow_default_covariance_fallback: true}
      params: {default_covariance: [4.0], cov_burn_in_steps: [20]}
      params_from_upstream:
        mean: {upstream: post_mean}
  comparison:
    name: post_like
    model:
      likelihood: {type: normal}
      params: {covariance_matrix: [0.8]}
      params_from_upstream:
        mean: {upstream: sim}
    data: {partition_name: stream}
    window:
      partitions:
      - partition:
          name: sim
          iteration: {type: ornstein_uhlenbeck}
          params: {thetas: [0.7], mus: [0.0], sigmas: [0.1]}
          init_state_values: [1.2]
          state_history_depth: 1
          seed: 4
        outside_upstreams:
          mus: {upstream: post_sampler}
      data: [{partition_name: stream}]
      depth: 12
    window_data_history_depth: {stream: 12}
  past_discount: 0.99
  memory_depth: 12
  seed: 77
`

const regressionMacro = `- type: scalar_regression_stats
  name: ols
  y: {partition_name: pair, value_indices: [1]}
  x: {partition_name: pair, value_indices: [0]}
  mode: window
  window_length: 6
  min_denominator: 1.0e-9
  state_history_depth: 3
`

func TestExpandInferenceMacros(t *testing.T) {
	for _, path := range []string{
		"../../cfg/example_posterior_macro_config.yaml",
		"../../cfg/example_regression_config.yaml",
		"../../.claude/skills/stochadex-model/recipes/posterior_estimation.yaml",
		"../../.claude/skills/stochadex-model/recipes/scalar_regression_stats.yaml",
		"testdata/gradient/fit_macro.yaml",
	} {
		t.Run(path+" runs alike expanded", func(t *testing.T) {
			expandedAlike(t, readFile(t, path))
		})
	}

	t.Run("a likelihood comparison over a window stepping its own model, with a burn-in", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+comparisonMacro)
	})

	t.Run("a posterior whose sampler drives a window partition, with variances only", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+posteriorMacro)
	})

	t.Run("a windowed regression without an intercept over a two-wide input", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+regressionMacro)
	})

	t.Run("comparison models stated only in params, or only by wiring", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+strings.Replace(comparisonMacro,
			"    params: {covariance_matrix: [0.8]}\n    params_from_upstream:\n      mean: {upstream: sim}\n",
			"    params: {covariance_matrix: [0.8], mean: [1.4]}\n", 1))
		wired := strings.NewReplacer(
			"    params: {covariance_matrix: [0.8]}\n    params_from_upstream:\n      mean: {upstream: sim}\n",
			"    params_from_upstream:\n      mean: {upstream: sim}\n      covariance_matrix: {upstream: noise}\n",
			"    partitions:\n", "    partitions:\n    - partition: {name: noise, iteration: {type: constant_values}, params: {}, init_state_values: [0.8], state_history_depth: 1, seed: 0}\n",
		).Replace(comparisonMacro)
		expandedAlike(t, inferenceInputs+wired)
	})

	t.Run("a posterior with a full covariance and a sampler wired entirely from upstream", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+strings.NewReplacer(
			"default: [4.0], just_variance: true", "default: [4.0]",
			"      params: {default_covariance: [4.0], cov_burn_in_steps: [20]}\n", "",
			"        mean: {upstream: post_mean}\n", "        mean: {upstream: post_mean}\n        covariance_matrix: {upstream: post_var}\n",
		).Replace(posteriorMacro))
	})

	t.Run("a fit whose model takes its covariance from a window partition", func(t *testing.T) {
		expandedAlike(t, strings.NewReplacer(
			"    params: {covariance_matrix: [1.0, 0.0, 0.0, 1.0]}\n",
			"    params_from_upstream: {covariance_matrix: {upstream: noise}}\n",
			"window: {depth: 10}", "window: {depth: 10, partitions: [{partition: {name: noise, iteration: {type: constant_values}, "+
				"params: {}, init_state_values: [1.0, 0.0, 0.0, 1.0], state_history_depth: 1, seed: 0}}]}",
		).Replace(readFile(t, "testdata/gradient/fit_macro.yaml")))
	})

	t.Run("a window model reading the data's previous row starts from the window's first", func(t *testing.T) {
		lagged := strings.NewReplacer(
			"      mean: {upstream: sim}\n", "      mean: {upstream: lagged}\n",
			"    partitions:\n", "    partitions:\n    - partition: {name: lagged, iteration: {type: expression, "+
				"fields: [{name: v}], upstreams: {previous: stream}, outputs: [\"previous\"]}, params: {}, "+
				"init_state_values: [0.0], state_history_depth: 1, seed: 0}\n",
		).Replace(comparisonMacro)
		expandedAlike(t, inferenceInputs+lagged)
	})

	t.Run("a regression on a constant regressor falls back to its minimum denominator", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+strings.Replace(regressionMacro,
			"x: {partition_name: pair, value_indices: [0]}", "x: {partition_name: flat}", 1))
	})

	t.Run("a mean-function fit warm-started from its last answer", func(t *testing.T) {
		yaml := strings.Replace(readFile(t, "testdata/gradient/fit_macro.yaml"),
			"  descent_iterations: 3\n", "  descent_iterations: 3\n  warm_start: true\n", 1)
		expandedAlike(t, yaml)
	})

	t.Run("a later macro reads a posterior's mean", func(t *testing.T) {
		expandedAlike(t, inferenceInputs+posteriorMacro+
			"- {type: vector_mean, name: smoothed, data: {partition_name: post_mean}, window: 4}\n")
	})

	cases := []struct{ name, yaml, from, to, want string }{
		{"a window of no depth", comparisonMacro, "    depth: 10\n", "    depth: 0\n", "window depth must be >= 1, got 0"},
		{"window data read by index", comparisonMacro, "    data: [{partition_name: stream}]",
			"    data: [{partition_name: stream, value_indices: [0]}]", "value indices are not supported in window data"},
		{"a window datum with no history depth", comparisonMacro, "  window_data_history_depth: {stream: 10}",
			"  window_data_history_depth: {level: 10}", `window_data_history_depth has no entry for window data partition "stream"`},
		{"a window datum with too little history", comparisonMacro, "  window_data_history_depth: {stream: 10}",
			"  window_data_history_depth: {stream: 9}", `window data partition "stream" has history depth 9, below the window's depth 10`},
		{"a negative burn-in", comparisonMacro, "embedded_burn_in_steps: 4", "embedded_burn_in_steps: -1",
			"embedded_burn_in_steps must be >= 0, got -1"},
		{"a comparison against data nothing makes", comparisonMacro, "  data: {partition_name: stream}\n",
			"  data: {partition_name: nothing}\n", `no partition "nothing" to read`},
		{"window data nothing makes", comparisonMacro,
			"    data: [{partition_name: stream}]\n    depth: 10\n  embedded_burn_in_steps: 4\n  window_data_history_depth: {stream: 10}\n",
			"    data: [{partition_name: nothing}]\n    depth: 10\n  embedded_burn_in_steps: 4\n", `no partition "nothing" to read`},
		{"a model with no likelihood", comparisonMacro, "    likelihood: {type: normal}\n    params: {covariance_matrix: [0.8]}",
			"    params: {covariance_matrix: [0.8]}", "model needs a likelihood"},
		{"a window partition with no iteration", comparisonMacro, "        iteration: {type: ornstein_uhlenbeck}\n", "",
			`window partition "sim" needs an iteration: {type: ...}`},
		{"a posterior with no mean", posteriorMacro, "mean: {name: post_mean, default: [0.0]}",
			"mean: {name: post_mean, default: []}", "posterior mean.default must be non-empty"},
		{"a sampler of the wrong width", posteriorMacro, "    default: [0.0]\n    distribution:",
			"    default: [0.0, 0.0]\n    distribution:", "sampler.default length must match the mean's 1, got 2"},
		{"variances of the wrong width", posteriorMacro, "default: [4.0], just_variance: true",
			"default: [4.0, 4.0], just_variance: true", "covariance.just_variance needs default of length 1, got 2"},
		{"a covariance of the wrong size", posteriorMacro, "default: [4.0], just_variance: true",
			"default: [4.0, 0.0]", "covariance.default must have length 1 (N²), got 2"},
		{"a posterior whose comparison does not expand", posteriorMacro, "      depth: 12\n", "      depth: 0\n",
			"window depth must be >= 1, got 0"},
		{"no memory", posteriorMacro, "memory_depth: 12", "memory_depth: 0", "memory_depth must be >= 1, got 0"},
		{"a posterior whose loglike ignores the sampler", posteriorMacro, "          mus: {upstream: post_sampler}",
			"          mus: {upstream: level}", `posterior comparison must read the sampler "post_sampler"`},
		{"a sampler with no likelihood", posteriorMacro, "      likelihood: {type: normal, allow_default_covariance_fallback: true}\n", "",
			"sampler.distribution needs a likelihood"},
		{"an unknown regression mode", regressionMacro, "mode: window", "mode: rolling",
			`scalar_regression_stats: unknown mode "rolling"`},
		{"a windowed regression with no window", regressionMacro, "window_length: 6", "window_length: 0",
			"window_length must be >= 1 in window mode, got 0"},
		{"a regression on two values", regressionMacro, "y: {partition_name: pair, value_indices: [1]}",
			"y: {partition_name: pair}", `y and x must each read exactly one value; "pair" gives 2`},
		{"a regression on a missing x", regressionMacro, "x: {partition_name: pair, value_indices: [0]}",
			"x: {partition_name: gone}", `no partition "gone" to read`},
	}
	for _, c := range cases {
		t.Run(c.name+" does not expand, saying why", func(t *testing.T) {
			macro := strings.Replace(c.yaml, c.from, c.to, 1)
			if macro == c.yaml {
				t.Fatalf("the case did not change the macro: %q", c.from)
			}
			config, err := LoadConfig(writeConfigPath(t, inferenceInputs+macro))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Expand(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	fit := readFile(t, "testdata/gradient/fit_macro.yaml")
	for _, c := range []struct{ name, from, to, want string }{
		{"an unknown gradient function", "function: mean_gradient", "function: slope", `gradient: unknown function "slope"`},
		{"a fit model with no likelihood", "    likelihood: {type: normal}\n    params:", "    params:", "model needs a likelihood"},
		{"a fit window of no depth", "window: {depth: 10}", "window: {depth: 0}", "window depth must be >= 1, got 0"},
		{"a fit with a negative burn-in", "  descent_iterations: 3\n", "  descent_iterations: 3\n  embedded_burn_in_steps: -2\n",
			"embedded_burn_in_steps must be >= 0, got -2"},
		{"a fit to data nothing makes", "  data: {partition_name: test_data}\n", "  data: {partition_name: absent}\n",
			`no partition "absent" to read`},
	} {
		t.Run(c.name+" does not expand, saying why", func(t *testing.T) {
			yaml := strings.Replace(fit, c.from, c.to, 1)
			if yaml == fit {
				t.Fatalf("the case did not change the macro: %q", c.from)
			}
			config, err := LoadConfig(writeConfigPath(t, yaml))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Expand(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
