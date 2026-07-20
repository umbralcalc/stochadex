package api

import (
	"math"
	"testing"
)

// TestSMCInferenceMacro checks the smc_inference macro's per-particle template
// recovers the true mean (2.0) of an observed data stream — a full particle-filter
// inference expressed entirely as config.
func TestSMCInferenceMacro(t *testing.T) {
	const cfg = `data:
  steps: 60
  timestep: 1.0
  partitions:
  - name: obs
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [2.0], covariance_matrix: [0.5]}
    init_state_values: [2.0]
    state_history_depth: 1
    seed: 7
macros:
- type: smc_inference
  proposal_name: smc_proposals
  sim_name: smc_sim
  posterior_name: smc_posterior
  num_particles: 100
  num_rounds: 3
  seed: 42
  priors: [{type: uniform, lo: -5.0, hi: 10.0}]
  param_names: [mean]
  model:
    observed_data: {name: observed_data, ref: {partition_name: obs}}
    per_particle_partitions:
    - name: "pred_{particle}"
      iteration: {type: param_values}
      params: {param_values: [0.0]}
      init_state_values: [0.0]
      state_history_depth: 2
    - name: "loglike_{particle}"
      iteration: {type: data_comparison, likelihood: {type: normal}}
      params: {mean: [0.0], variance: [0.5], latest_data_values: [2.0], cumulative: [1], burn_in_steps: [0]}
      params_from_upstream:
        mean: {upstream: "pred_{particle}"}
        latest_data_values: {upstream: observed_data}
      init_state_values: [0.0]
      state_history_depth: 2
    loglike_partition: "loglike_{particle}"
    param_forwarding:
      "pred_{particle}/param_values": [0]
`
	out := runMacroConfig(t, cfg)
	post := out["smc_posterior"]
	if len(post) == 0 {
		t.Fatal("no smc_posterior output")
	}
	// Posterior state layout begins with the mean estimate.
	if got := post[len(post)-1][0]; math.Abs(got-2.0) > 0.5 {
		t.Errorf("SMC posterior mean = %v, want ~2.0", got)
	}
}

// TestEvolutionStrategyMacro checks the live evolution-strategy macro runs (with
// no data: block) and, on the fully-data path (an {type: expression} reward), the
// mean update converges on the reward's known optimum. The objective is the
// negative squared distance from [3, -2], so the optimum is [3, -2] itself. This
// exercises the same convergence the Go integration test guards, but end-to-end
// through YAML: it would fail on the covariance divergence and init-placeholder
// bugs, or if the covariance contracted before the mean reached the optimum.
func TestEvolutionStrategyMacro(t *testing.T) {
	const cfg = `macros:
- type: evolution_strategy_optimisation
  steps: 1000
  seed: 12345
  sampler: {name: test_sampler, default: [0.0, 0.0]}
  sorting: {name: test_sorting, collection_size: 10, empty_value: -9999.0}
  mean: {name: test_mean, default: [0.0, 0.0], weights: [0.5, 0.3, 0.2], learning_rate: 0.5}
  covariance: {name: test_covariance, default: [4.0, 0.0, 0.0, 4.0], learning_rate: 0.1}
  reward:
    discount_factor: 0.0
    partition:
      partition:
        name: reward
        iteration:
          type: expression
          fields: [{name: r}]
          outputs: ["-((sample_values[0]-3)*(sample_values[0]-3) + (sample_values[1]+2)*(sample_values[1]+2))"]
        params: {sample_values: [0.0, 0.0]}
        init_state_values: [0.0]
        state_history_depth: 1
        seed: 0
      outside_upstreams: {sample_values: {upstream: test_sampler}}
  window:
    depth: 5
    partitions:
    - partition: {name: sim_partition, iteration: {type: constant_values}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
`
	out := runMacroConfig(t, cfg)
	means := out["test_mean"]
	if len(means) == 0 {
		t.Fatal("evolution strategy produced no test_mean output")
	}
	final := means[len(means)-1]
	target := []float64{3.0, -2.0}
	for i, want := range target {
		if math.Abs(final[i]-want) > 1e-2 {
			t.Errorf("test_mean did not converge: got %v, want %v", final, target)
			break
		}
	}
}
