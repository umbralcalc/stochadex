package api

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestMacroRunLeavesConfigUnmodified checks RunToStorage's promise that the
// caller's config is not modified, for every macro type in the shipped configs.
// A macro hands parts of its spec — params maps above all — to the partitions it
// builds, and the run writes resolved wiring into those partitions' params
// (params_as_partitions indices, params_from_upstream values). Shared rather than
// copied, that wiring leaks back into config.Macros, where a second run, an
// expansion or a re-serialisation then reads it as if the author had written it.
// Reference: the config's own YAML encoding, taken before the run.
func TestMacroRunLeavesConfigUnmodified(t *testing.T) {
	for _, path := range []string{
		"../../cfg/example_macro_config.yaml",
		"../../cfg/example_posterior_macro_config.yaml",
		"../../cfg/example_regression_config.yaml",
		"../../cfg/example_smc_config.yaml",
		"../../cfg/example_state_space_count_config.yaml",
		"../../cfg/example_evolution_strategy_config.yaml",
		"../../cfg/example_mcts_config.yaml",
		"../../cfg/example_planning_config.yaml",
		"testdata/gradient/fit_macro.yaml",
		"testdata/grouped/grouped_macro.yaml",
		"testdata/replicated/smc_macro.yaml",
		"testdata/planning/chain_macro.yaml",
	} {
		t.Run(path, func(t *testing.T) {
			config := LoadApiRunConfigFromYaml(path)
			before := encodeMacros(t, config)
			if _, err := RunToStorage(config); err != nil {
				t.Fatal(err)
			}
			if after := encodeMacros(t, config); after != before {
				t.Errorf("running the macros modified the config:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
	t.Run("a just_variance posterior, whose sampler is wired to its variance partition", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, justVariancePosterior))
		if err != nil {
			t.Fatal(err)
		}
		before := encodeMacros(t, config)
		if _, err := RunToStorage(config); err != nil {
			t.Fatal(err)
		}
		if after := encodeMacros(t, config); after != before {
			t.Errorf("running the macros modified the config:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})
}

func encodeMacros(t *testing.T, config *ApiRunConfig) string {
	t.Helper()
	encoded, err := yaml.Marshal(config.Macros)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// justVariancePosterior estimates an OU process's reversion level from a normal
// stream, with a variances-only posterior and a sampler that falls back to a
// fixed proposal during burn-in.
const justVariancePosterior = `inputs:
  data:
    simulation:
      steps: 60
      timestep: 1.0
      partitions:
      - name: stream
        iteration: {type: data_generation, likelihood: {type: normal}}
        params: {mean: [1.5], covariance_matrix: [0.8]}
        init_state_values: [1.2]
        state_history_depth: 1
        seed: 17
macros:
- type: posterior_estimation
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
