package api

import "testing"

// TestJustVariancePosteriorRunLeavesConfigUnmodified checks a variances-only
// posterior, whose sampler the macro wires to its variance partition by
// params_as_partitions, keeps that wiring out of the config: a run resolves it to
// a partition index in the sampler's params, which a later Expand would otherwise
// carry as a literal (see TestMacroRunLeavesConfigUnmodified for every macro type).
func TestJustVariancePosteriorRunLeavesConfigUnmodified(t *testing.T) {
	config, err := LoadConfig(writeConfigPath(t, justVariancePosterior))
	if err != nil {
		t.Fatal(err)
	}
	before := marshalledMacroTiers(t, config)
	if _, err := RunToStorage(config); err != nil {
		t.Fatal(err)
	}
	if after := marshalledMacroTiers(t, config); after != before {
		t.Errorf("running the macros modified the config:\nbefore:\n%s\nafter:\n%s", before, after)
	}
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
