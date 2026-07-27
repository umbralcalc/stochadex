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
