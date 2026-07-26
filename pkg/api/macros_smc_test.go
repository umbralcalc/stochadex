package api

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestModelPartitionInstantiation guards SMC correctness at the mechanism level:
// every particle gets its own instance of each model partition, with its own
// params and wiring maps. Sharing any of them would couple particles, since
// upstream injection writes into params at runtime.
func TestModelPartitionInstantiation(t *testing.T) {
	template := simulator.PartitionConfig{
		Name:            "pred",
		Params:          simulator.NewParams(map[string][]float64{"param_values": {0.0}}),
		InitStateValues: []float64{0.0},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"mean": {Upstream: "pred"},
		},
		ParamsAsPartitions: map[string][]string{
			"peer": {"loglike"},
		},
	}

	p0, err := instantiateModelPartition(template)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := instantiateModelPartition(template)
	if err != nil {
		t.Fatal(err)
	}

	// Names are no longer templated: each particle runs its own simulation, so
	// two particles' partitions can share a name without colliding.
	if p0.Name != "pred" || p1.Name != "pred" {
		t.Errorf("names should be verbatim: %q, %q", p0.Name, p1.Name)
	}
	if p0.ParamsFromUpstream["mean"].Upstream != "pred" {
		t.Errorf("upstream should be verbatim: %q", p0.ParamsFromUpstream["mean"].Upstream)
	}
	if p1.ParamsAsPartitions["peer"][0] != "loglike" {
		t.Errorf("params_as_partitions should be verbatim: %v", p1.ParamsAsPartitions["peer"])
	}

	p0.Params.Map["param_values"][0] = 99.0
	if p1.Params.Map["param_values"][0] != 0.0 {
		t.Error("params not deep-copied: particle 1 saw particle 0's mutation")
	}
	if template.Params.Map["param_values"][0] != 0.0 {
		t.Error("params not deep-copied: the template was mutated")
	}
	p0.ParamsAsPartitions["peer"][0] = "elsewhere"
	if p1.ParamsAsPartitions["peer"][0] != "loglike" {
		t.Error("params_as_partitions not deep-copied between particles")
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
// can only come from their random streams. Particles sharing one realisation is
// invisible with a deterministic model and fatal for simulation-based inference,
// where the cloud is supposed to carry the model's Monte Carlo variation.
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
    partitions:
    - name: "walk"
      iteration: {type: wiener_process}
      params: {variances: [1.0]}
      init_state_values: [0.0]
      state_history_depth: 2
      seed: 99
    - name: "loglike"
      iteration: {type: data_comparison, likelihood: {type: normal}}
      params: {mean: [0.0], variance: [0.5], latest_data_values: [2.0], cumulative: [1], burn_in_steps: [0]}
      params_from_upstream:
        mean: {upstream: "walk"}
        latest_data_values: {upstream: observed_data}
      init_state_values: [0.0]
      state_history_depth: 2
    loglike_partition: "loglike"
`
	rows := runMacroConfig(t, cfg)["smc_sim"]
	// The evaluation partition's row is one log-likelihood per particle.
	loglikes := rows[len(rows)-1]
	if len(loglikes) != 4 {
		t.Fatalf("expected one log-likelihood per particle, got %v", loglikes)
	}
	for i, value := range loglikes {
		for j := i + 1; j < len(loglikes); j++ {
			if value == loglikes[j] {
				t.Fatalf("particles %d and %d share a noise realisation (both %v); "+
					"per-particle seeds are not being varied: %v", i, j, value, loglikes)
			}
		}
	}
}
