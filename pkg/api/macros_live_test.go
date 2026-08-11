package api

import (
	"math"
	"os"
	"strings"
	"testing"
)

// TestStateSpaceCountCalibration pins the shipped state-space count recipe
// (cfg/example_state_space_count_config.yaml): a latent-intensity → dispersion-aware
// count likelihood → SMC posterior model, expressed entirely as config with no new
// engine code. It recovers BOTH the mean (8) and the DISPERSION (2) of an
// over-dispersed negative-binomial stream — and, run with a Poisson observation
// likelihood instead, the mean still recovers but the dispersion drifts to its prior
// mean, because Poisson forces Var = mean so the dispersion never enters the
// log-likelihood. That contrast is the point: the composition is only useful with a
// dispersion-aware family, and the engine already ships one. This reproduces, domain-
// free, the identifiability result the cryptobook project recorded (and is why the
// STOCHADEX_GAPS "state-space likelihood" note is a composition, not a missing feature).
func TestStateSpaceCountCalibration(t *testing.T) {
	raw, err := os.ReadFile("../../cfg/example_state_space_count_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	negBinom := string(raw)

	t.Run("negative_binomial recovers mean and dispersion", func(t *testing.T) {
		post := runMacroConfig(t, negBinom)["smc_posterior"]
		if len(post) == 0 {
			t.Fatal("no smc_posterior output")
		}
		last := post[len(post)-1]
		if math.Abs(last[0]-8.0) > 2.0 {
			t.Errorf("mean = %v, want ~8", last[0])
		}
		if math.Abs(last[1]-2.0) > 1.5 {
			t.Errorf("dispersion = %v, want ~2 (the parameter Poisson cannot see)", last[1])
		}
	})

	t.Run("poisson recovers the mean but not the dispersion", func(t *testing.T) {
		poisson := strings.Replace(
			negBinom,
			`iteration: {type: data_comparison, likelihood: {type: negative_binomial}}`,
			`iteration: {type: data_comparison, likelihood: {type: poisson}}`,
			1,
		)
		if poisson == negBinom {
			t.Fatal("failed to swap the observation likelihood — recipe wording changed")
		}
		post := runMacroConfig(t, poisson)["smc_posterior"]
		last := post[len(post)-1]
		if math.Abs(last[0]-8.0) > 2.0 {
			t.Errorf("mean = %v, want ~8 (the mean is still identifiable under Poisson)", last[0])
		}
		if math.Abs(last[1]-2.0) < 1.0 {
			t.Errorf("dispersion = %v recovered under Poisson, but it must NOT — "+
				"Poisson forces Var=mean so the dispersion is unidentified", last[1])
		}
	})
}

// TestSMCInferenceMacro checks the smc_inference macro's per-particle model
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
    partitions:
    - name: "pred"
      iteration: {type: param_values}
      params: {param_values: [0.0]}
      init_state_values: [0.0]
      state_history_depth: 2
    - name: "loglike"
      iteration: {type: data_comparison, likelihood: {type: normal}}
      params: {mean: [0.0], variance: [0.5], latest_data_values: [2.0], cumulative: [1], burn_in_steps: [0]}
      params_from_upstream:
        mean: {upstream: "pred"}
        latest_data_values: {upstream: observed_data}
      init_state_values: [0.0]
      state_history_depth: 2
    loglike_partition: "loglike"
    param_forwarding:
      "pred/param_values": [0]
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
