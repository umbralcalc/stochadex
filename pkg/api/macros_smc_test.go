package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestParticleTemplateInstantiation guards SMC correctness at the mechanism level:
// each particle must get its OWN partition with {particle} substituted through the
// name and every upstream reference, and its OWN params map — otherwise particles
// silently share state and the inference is quietly wrong.
func TestParticleTemplateInstantiation(t *testing.T) {
	template := simulator.PartitionConfig{
		Name:            "pred_{particle}",
		Params:          simulator.NewParams(map[string][]float64{"param_values": {0.0}}),
		InitStateValues: []float64{0.0},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"mean": {Upstream: "pred_{particle}"},
		},
		ParamsAsPartitions: map[string][]string{
			"peer": {"loglike_{particle}"},
		},
	}

	p0, err := instantiateParticle(template, 0)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := instantiateParticle(template, 1)
	if err != nil {
		t.Fatal(err)
	}

	if p0.Name != "pred_0" || p1.Name != "pred_1" {
		t.Errorf("names not substituted: %q, %q", p0.Name, p1.Name)
	}
	if p0.ParamsFromUpstream["mean"].Upstream != "pred_0" {
		t.Errorf("upstream not substituted: %q", p0.ParamsFromUpstream["mean"].Upstream)
	}
	if p1.ParamsAsPartitions["peer"][0] != "loglike_1" {
		t.Errorf("params_as_partitions not substituted: %v", p1.ParamsAsPartitions["peer"])
	}

	// Deep-copy invariant: mutating one particle's params must not touch the other's
	// or the template's — upstream injection writes into params at runtime.
	p0.Params.Map["param_values"][0] = 99.0
	if p1.Params.Map["param_values"][0] != 0.0 {
		t.Error("params not deep-copied: particle 1 saw particle 0's mutation")
	}
	if template.Params.Map["param_values"][0] != 0.0 {
		t.Error("params not deep-copied: the template was mutated")
	}
}

// TestLiveMacroRejectsAgainstStorage checks the live macros error clearly if run
// through the against-storage path, rather than misbehaving.
func TestLiveMacroRejectsAgainstStorage(t *testing.T) {
	for _, spec := range []macroSpec{&evolutionStrategySpec{}, &smcInferenceSpec{}} {
		if _, _, err := spec.resolve(nil); err == nil {
			t.Errorf("%T.resolve should error (it is a live macro)", spec)
		}
	}
}

// TestSMCParticlesGetDistinctNoise pins per-particle seeding. The model here is
// stochastic and reads no proposal parameters, so any variation between particles
// can only come from their random streams.
//
// Before this was fixed, per-particle partitions inherited the template's seed
// verbatim and every particle ran the same noise realisation — invisible with a
// deterministic model, and silently fatal for simulation-based inference, where
// the particle cloud is supposed to carry the model's Monte Carlo variation.
func TestSMCParticlesGetDistinctNoise(t *testing.T) {
	const cfg = `data:
  steps: 12
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
  num_particles: 4
  num_rounds: 1
  seed: 42
  priors: [{type: uniform, lo: -5.0, hi: 10.0}]
  param_names: [mean]
  model:
    observed_data: {name: observed_data, ref: {partition_name: obs}}
    per_particle_partitions:
    - name: "walk_{particle}"
      iteration: {type: wiener_process}
      params: {variances: [1.0]}
      init_state_values: [0.0]
      state_history_depth: 2
      seed: 99
    - name: "loglike_{particle}"
      iteration: {type: data_comparison, likelihood: {type: normal}}
      params: {mean: [0.0], variance: [0.5], latest_data_values: [2.0], cumulative: [1], burn_in_steps: [0]}
      params_from_upstream:
        mean: {upstream: "walk_{particle}"}
        latest_data_values: {upstream: observed_data}
      init_state_values: [0.0]
      state_history_depth: 2
    loglike_partition: "loglike_{particle}"
`
	rows := runMacroConfig(t, cfg)["smc_sim"]
	final := rows[len(rows)-1]
	// Inner layout: observed_data(1), then per particle walk(1), loglike(1).
	endpoints := []float64{final[1], final[3], final[5], final[7]}
	for i, value := range endpoints {
		for j := i + 1; j < len(endpoints); j++ {
			if value == endpoints[j] {
				t.Fatalf("particles %d and %d share a noise realisation (both %v); "+
					"per-particle seeds are not being varied: %v", i, j, value, endpoints)
			}
		}
	}
}
