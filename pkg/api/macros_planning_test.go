package api

import (
	"strings"
	"testing"
)

// planningConfig is battery arbitrage over a cyclic price, written entirely as
// config: two model partitions for the dynamics, one expression partition for the
// reward, and a discrete charge/hold/discharge action set.
//
// The optimum is knowable by hand. The price cycles 10, 10, 90, 90 and the
// battery holds one unit, so the best play is to buy on a cheap step and sell on
// a dear one — twice over an 8-step run, for 2 * (90 - 10) = 160. Doing nothing
// earns 0, and no myopic rule reaches 160, because buying looks like a pure loss
// at the moment it has to happen.
const planningConfig = `macros:
- type: mcts_planning
  name: plan
  steps: 8
  horizon: 8
  sims_per_decision: 400
  seed: 99
  return_range: [-400, 400]
  actions: [[1], [0], [-1]]
  action_partition: battery
  action_param: dispatch
  reward_partition: revenue
  model:
    partitions:
    - name: price
      iteration:
        type: expression
        fields: [{name: price}, {name: phase}]
        bindings:
        - {name: nextphase, expr: "phase + 1 - 4 * floor((phase + 1) / 4)"}
        outputs: ["where(nextphase < 2, 10, 90)", "nextphase"]
      init_state_values: [90.0, 3.0]
      state_history_depth: 1
      seed: 0
    - name: battery
      iteration:
        type: expression
        fields: [{name: soc}, {name: flow}]
        bindings:
        - {name: nextsoc, expr: "clamp(soc + dispatch, 0, 1)"}
        outputs: ["nextsoc", "nextsoc - soc"]
      params: {dispatch: [0.0]}
      init_state_values: [0.0, 0.0]
      state_history_depth: 1
      seed: 0
    - name: revenue
      iteration:
        type: expression
        fields: [{name: r}]
        outputs: ["0 - flow * price"]
      params: {flow: [0.0], price: [0.0]}
      params_from_upstream:
        flow: {upstream: battery, indices: [1]}
        price: {upstream: price, indices: [0]}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
`

// TestMCTSPlanningMacro is the end-to-end check that a planning run is
// expressible as data: no Go, no registered environment, just a forward model
// and an action set. The assertion is on the plan's value, not that it ran.
func TestMCTSPlanningMacro(t *testing.T) {
	out := runMacroConfig(t, planningConfig)
	rows := out["plan_apply"]
	if len(rows) < 2 {
		t.Fatalf("expected the driven model to be recorded, got %d rows", len(rows))
	}
	// Encoded state layout: [step, accumulated return, price(2), battery(2), revenue(1)].
	final := rows[len(rows)-1]
	planned := final[1]
	t.Logf("planned return %.1f over %v steps (final state %v)", planned, final[0], final)

	if planned < 120 {
		t.Errorf("planned return %.1f: the search is not exploiting the price cycle "+
			"(doing nothing earns 0, the optimum is 160)", planned)
	}
	if planned > 160.0001 {
		t.Errorf("planned return %.1f exceeds the achievable optimum of 160", planned)
	}
}

// TestMCTSPlanningMacroValidation checks the macro rejects the config mistakes
// that would otherwise produce a plausible-looking but meaningless plan — most
// importantly a reward or action partition that does not exist, which would
// silently optimise nothing.
func TestMCTSPlanningMacroValidation(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		replace    [2]string
		wantErrHas string
	}{
		{
			name:       "unknown reward partition",
			replace:    [2]string{"reward_partition: revenue", "reward_partition: profit"},
			wantErrHas: `no model partition named "profit"`,
		},
		{
			name:       "unknown action partition",
			replace:    [2]string{"action_partition: battery", "action_partition: turbine"},
			wantErrHas: `no model partition named "turbine"`,
		},
		{
			name:       "missing return range",
			replace:    [2]string{"return_range: [-400, 400]", "return_range: [0]"},
			wantErrHas: "return_range",
		},
		{
			name:       "no actions",
			replace:    [2]string{"actions: [[1], [0], [-1]]", "actions: []"},
			wantErrHas: "actions",
		},
		{
			name:       "no horizon",
			replace:    [2]string{"horizon: 8", "horizon: 0"},
			wantErrHas: "horizon",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := strings.Replace(
				planningConfig, testCase.replace[0], testCase.replace[1], 1)
			if cfg == planningConfig {
				t.Fatalf("test setup: %q not found in the config", testCase.replace[0])
			}
			err := macroConfigError(t, cfg)
			if err == nil {
				t.Fatalf("expected an error mentioning %q", testCase.wantErrHas)
			}
			if !strings.Contains(err.Error(), testCase.wantErrHas) {
				t.Errorf("error should mention %q, got: %v", testCase.wantErrHas, err)
			}
		})
	}
}

// TestMCTSPlanningProgressPartition covers the progress proxy's config surface:
// a named partition is used to score rollouts that truncate, and a typo is an
// error rather than a silently unscored search.
func TestMCTSPlanningProgressPartition(t *testing.T) {
	t.Run("a named progress partition resolves", func(t *testing.T) {
		cfg := strings.Replace(planningConfig,
			"  reward_partition: revenue",
			"  reward_partition: revenue\n  progress_partition: revenue", 1)
		if cfg == planningConfig {
			t.Fatal("test setup: reward_partition not found")
		}
		if err := macroConfigError(t, cfg); err != nil {
			t.Fatalf("progress_partition should resolve: %v", err)
		}
	})

	t.Run("an unknown progress partition is an error", func(t *testing.T) {
		cfg := strings.Replace(planningConfig,
			"  reward_partition: revenue",
			"  reward_partition: revenue\n  progress_partition: absent", 1)
		err := macroConfigError(t, cfg)
		if err == nil || !strings.Contains(err.Error(), `no model partition named "absent"`) {
			t.Errorf("expected a named error for an unknown progress partition, got: %v", err)
		}
	})
}

// TestMCTSPlanningOverAPosterior is posterior-predictive planning as config: the
// data: tier records a spread of parameter draws, and the planner averages over
// them instead of committing to a point estimate.
//
// The model is the newsvendor, where the two provably disagree. Demand is 10 in
// five draws of six and 100 in the sixth, so its mean is 25. Ordering 25 is the
// best response to demand=25 (profit 100) but loses money against the actual
// spread (−25), while ordering 10 earns a certain 40.
func TestMCTSPlanningOverAPosterior(t *testing.T) {
	const cfg = `data:
  steps: 5
  timestep: 1.0
  partitions:
  - name: demand_draws
    iteration:
      type: expression
      fields: [{name: d}]
      bindings:
      - {name: phase, expr: "step - 6 * floor(step / 6)"}
      outputs: ["where(phase < 5, 10, 100)"]
    init_state_values: [10.0]
    state_history_depth: 1
    seed: 0
macros:
- type: mcts_planning
  name: plan
  steps: 2
  horizon: 1
  sims_per_decision: 600
  seed: 11
  return_range: [-700, 300]
  actions: [[10], [25], [100]]
  action_partition: order
  action_param: param_values
  reward_partition: profit
  parameters:
    samples_from: {partition_name: demand_draws}
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
      params: {quantity: [0.0], demand: [25.0]}
      params_from_upstream:
        quantity: {upstream: order}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
`
	out := runMacroConfig(t, cfg)
	rows := out["plan_apply"]
	if len(rows) < 2 {
		t.Fatalf("expected the plan to be recorded, got %d rows", len(rows))
	}
	// The order actually placed shows up in the order partition's slot of the
	// encoded state: [step, return, order(1), profit(1), sample index]. The
	// first outer step is the sentinel one where the search has not yet produced
	// a decision, so the applied action appears from the second.
	ordered := rows[len(rows)-1][2]
	t.Logf("planning over the recorded posterior ordered %v", ordered)

	if ordered != 10 {
		t.Errorf("posterior-predictive planning ordered %v, want 10 — ordering 25 is only "+
			"right if demand really is its mean", ordered)
	}
}

// TestMCTSPlanningWithBeliefUpdating is value of information as config: the
// planner opens by probing, which pays nothing, because doing so tells it which
// commitment is right.
//
// theta decides which commitment pays and is equally likely to be either, so
// committing blind is worth zero. Only a planner that updates its belief from
// the probe's result has a reason to spend a step on it.
func TestMCTSPlanningWithBeliefUpdating(t *testing.T) {
	const cfg = `macros:
- type: mcts_planning
  name: plan
  steps: 2
  horizon: 2
  sims_per_decision: 800
  seed: 21
  return_range: [-30, 30]
  actions: [[0], [1], [2]]
  action_partition: choice
  action_param: param_values
  reward_partition: payoff
  parameters:
    samples: [[0.0], [1.0]]
    targets: [{partition: signal, param: theta, indices: [0]}, {partition: payoff, param: theta, indices: [0]}]
    belief: {observation_partition: signal, variance: 0.01}
  model:
    partitions:
    - name: choice
      iteration: {type: param_values}
      params: {param_values: [0.0]}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
    # Probing (choice 0) shows theta; anything else shows 0 whatever theta is,
    # so it teaches nothing.
    - name: signal
      iteration:
        type: expression
        fields: [{name: s}]
        outputs: ["where(choice < 0.5, theta, 0)"]
      params: {choice: [0.0], theta: [0.0]}
      params_from_upstream:
        choice: {upstream: choice}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
    # Commit A (choice 1) pays when theta is 0, commit B (choice 2) when it is 1.
    - name: payoff
      iteration:
        type: expression
        fields: [{name: p}]
        outputs: ["where(choice < 0.5, 0, where(choice < 1.5, 10 - 20 * theta, 0 - 10 + 20 * theta))"]
      params: {choice: [0.0], theta: [0.0]}
      params_from_upstream:
        choice: {upstream: choice}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
`
	out := runMacroConfig(t, cfg)
	rows := out["plan_apply"]
	if len(rows) < 3 {
		t.Fatalf("expected the plan to be recorded, got %d rows", len(rows))
	}
	// The first real decision lands on the second recorded ply (the first is the
	// sentinel step where the search has not yet produced one). The choice
	// partition is the first model partition, so it sits at offset 2.
	opening := rows[2][2]
	t.Logf("belief-updating planner opened with choice %v (0 = probe)", opening)

	if opening != 0 {
		t.Errorf("a planner that can learn should probe first, it chose %v", opening)
	}
}

// TestMCTSPlanningFromAnInferredPosterior is the chain the planning tier exists
// for: infer a parameter from data, then plan under what was inferred —
// uncertainty included — in one config with no Go.
//
// posterior_estimation already samples the running posterior each step, so its
// sampler partition's recorded rows ARE the posterior sample set. The planner
// reads them directly, past a burn-in that drops the rows produced while the
// estimate was still moving.
//
// The data has a mean of 3, recovered from an off-truth prior of 0. The decision
// is a newsvendor whose right order depends on that mean, so the plan is only
// right if the inference actually reached the truth.
func TestMCTSPlanningFromAnInferredPosterior(t *testing.T) {
	const cfg = `data:
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
    # The sampler's own draws, after the estimate has settled.
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
	// A live macro runs in its own context and replaces the storage, so the
	// posterior partitions are no longer readable by the time the plan is out.
	out := runMacroConfig(t, cfg)
	rows := out["plan_apply"]
	if len(rows) < 3 {
		t.Fatalf("expected the plan to be recorded, got %d rows", len(rows))
	}
	ordered := rows[len(rows)-1][2]
	t.Logf("planning under the inferred posterior ordered %v (demand truly averages 3)",
		ordered)

	// Ordering 3 suits demand near 3: 1 leaves profit unclaimed and 9 buys stock
	// that cannot be sold.
	if ordered != 3 {
		t.Errorf("ordered %v, want 3 — the plan should follow the inferred demand", ordered)
	}
}
