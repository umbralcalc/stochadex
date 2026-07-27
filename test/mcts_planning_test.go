package main

// Integration coverage for the planning tier: whole configs, resolved and run
// the way a user runs them, asserting on what the plan is worth rather than on
// whether it executed.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/api"
)

// runPlanningConfig writes a config, runs its macros, and returns the recorded
// output by partition name.
func runPlanningConfig(t *testing.T, yaml string) map[string][][]float64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	storage, err := api.RunMacros(api.LoadApiRunConfigFromYaml(path))
	if err != nil {
		t.Fatalf("running the planning config: %v", err)
	}
	out := make(map[string][][]float64)
	for _, name := range storage.GetNames() {
		out[name] = storage.GetValues(name)
	}
	return out
}

// TestPlanningRecoversAKnownOptimum runs the shipped battery-arbitrage example
// as a user would and checks the plan is worth what the problem's optimum is
// worth.
//
// The price cycles 10, 10, 90, 90 and the battery holds one unit, so buying
// cheap and selling dear twice is worth 2 * (90 - 10) = 160. Doing nothing earns
// zero, and no myopic rule gets there, because buying looks like a pure loss at
// the moment it has to happen.
func TestPlanningRecoversAKnownOptimum(t *testing.T) {
	config, err := os.ReadFile(filepath.Join("..", "cfg", "example_planning_config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rows := runPlanningConfig(t, string(config))["plan_apply"]
	if len(rows) < 2 {
		t.Fatalf("expected the driven model to be recorded, got %d rows", len(rows))
	}
	// Encoded state: [step, accumulated return, ...partition rows].
	earned := rows[len(rows)-1][1]
	t.Logf("the shipped planning example earned %v (optimum 160, doing nothing 0)", earned)

	if earned < 120 {
		t.Errorf("planned return %v: the search is not exploiting the price cycle", earned)
	}
	if earned > 160.0001 {
		t.Errorf("planned return %v exceeds the achievable optimum of 160", earned)
	}
}

// TestPlanningUnderAnInferredPosterior is the chain the planning tier exists
// for, run end to end as one config: learn a parameter from data, then decide
// under what was learned, uncertainty included.
//
// Demand averages 3 and posterior_estimation recovers it from a prior of 0. The
// decision is a newsvendor, whose right order depends on that parameter — so a
// correct plan is evidence the inference reached the truth AND that its spread
// reached the planner.
func TestPlanningUnderAnInferredPosterior(t *testing.T) {
	const config = `data:
  steps: 2500
  timestep: 1.0
  partitions:
  - name: observations
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [3.0], covariance_matrix: [0.25]}
    init_state_values: [3.0]
    state_history_depth: 2500
    seed: 17
macros:
- type: posterior_estimation
  log_norm: {name: post_log_norm, default: 0.0}
  mean: {name: post_mean, default: [0.0]}
  covariance: {name: post_cov, default: [4.0]}
  sampler:
    name: post_sampler
    default: [0.0]
    distribution:
      likelihood: {type: normal, allow_default_covariance_fallback: true}
      params: {default_covariance: [4.0], cov_burn_in_steps: [100]}
      params_from_upstream:
        mean: {upstream: post_mean}
        covariance_matrix: {upstream: post_cov}
  comparison:
    name: post_likelihood
    model:
      likelihood: {type: normal}
      params: {covariance_matrix: [0.25]}
      params_from_upstream:
        mean: {upstream: post_sampler}
    data: {partition_name: observations}
    window:
      data: [{partition_name: observations}]
      depth: 100
    window_data_history_depth: {observations: 100}
  past_discount: 0.999
  memory_depth: 100
  seed: 1234
- type: mcts_planning
  name: plan
  steps: 2
  horizon: 1
  sims_per_decision: 400
  seed: 3
  return_range: [-200, 200]
  actions: [[1], [3], [9]]
  action_partition: order
  action_param: param_values
  reward_partition: profit
  parameters:
    samples_from: {partition_name: post_sampler, burn_in: 2000}
    targets: [{partition: profit, param: demand, indices: [0]}]
  model:
    partitions:
    - name: order
      iteration: {type: param_values}
      params: {param_values: [0.0]}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
    - name: profit
      iteration:
        type: expression
        fields: [{name: p}]
        outputs: ["10 * min(quantity, demand) - 6 * quantity"]
      params: {quantity: [0.0], demand: [0.0]}
      params_from_upstream:
        quantity: {upstream: order}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
`
	rows := runPlanningConfig(t, config)["plan_apply"]
	if len(rows) < 3 {
		t.Fatalf("expected the plan to be recorded, got %d rows", len(rows))
	}
	ordered := rows[len(rows)-1][2]
	t.Logf("planning under the inferred posterior ordered %v (demand truly averages 3)", ordered)

	// Ordering 3 suits demand near 3: 1 leaves profit unclaimed, 9 buys stock
	// that cannot be sold.
	if ordered != 3 {
		t.Errorf("ordered %v, want 3 — the plan should follow the inferred demand", ordered)
	}
}

// TestSelfPlayRunsFromConfig covers the other half of the decision tier: a
// search over decision RULES, reached through the environment registry rather
// than stated as data.
func TestSelfPlayRunsFromConfig(t *testing.T) {
	config, err := os.ReadFile(filepath.Join("..", "cfg", "example_mcts_config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rows := runPlanningConfig(t, string(config))["ttt_apply"]
	if len(rows) < 3 {
		t.Fatalf("expected a game to be played out, got %d rows", len(rows))
	}
	first, last := rows[0], rows[len(rows)-1]
	same := true
	for i := range first {
		if first[i] != last[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("the encoded game state never changed; self-play did not advance the game")
	}
}
