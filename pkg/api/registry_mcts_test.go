package api

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
)

// runPair runs a macro config and its core-language twin.
func runPair(t *testing.T, macroYAML, twinYAML string) (*RunResult, *RunResult) {
	t.Helper()
	macro, err := RunToStorage(writeConfig(t, macroYAML))
	if err != nil {
		t.Fatal(err)
	}
	twin, err := RunToStorage(writeConfig(t, twinYAML))
	if err != nil {
		t.Fatal(err)
	}
	return macro, twin
}

func TestMCTSIterationSpecs(t *testing.T) {
	example := readFile(t, "../../cfg/example_planning_config.yaml")
	exampleTwin := readFile(t, "testdata/planning/example_twin.yaml")
	planning := readFile(t, "testdata/planning/planning_macro.yaml")
	planningTwin := readFile(t, "testdata/planning/planning_twin.yaml")

	t.Run("the shipped planning example, written out, plans the optimal 160 exactly as the macro does", func(t *testing.T) {
		macro, twin := runPair(t, example, exampleTwin)
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
		// The price cycles 10, 10, 90, 90 and the battery holds one unit, so
		// the best 8 steps buy cheap and sell dear twice: 2 * (90 - 10).
		rows := twin.Storage.GetValues("plan_apply")
		if banked := rows[len(rows)-1][1]; banked != 160 {
			t.Errorf("the twin banked %v, want the optimum 160", banked)
		}
	})

	t.Run("every environment field and search setting carries over exactly", func(t *testing.T) {
		macro, twin := runPair(t, planning, planningTwin)
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
	})

	t.Run("pinned noise and a scenario seed carry over exactly", func(t *testing.T) {
		macro, twin := runPair(t,
			strings.Replace(planning, "  sims_per_decision: 60\n",
				"  sims_per_decision: 60\n  pinned_noise: true\n  scenario_seed: 5\n", 1),
			strings.NewReplacer(
				"        type: simulation\n", "        type: simulation\n        scenario_seed: 5\n",
				"max_tree_depth: 5}", "max_tree_depth: 5, pinned_noise: true}",
			).Replace(planningTwin))
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
		chance, err := RunToStorage(writeConfig(t, planningTwin))
		if err != nil {
			t.Fatal(err)
		}
		if equalRows(chance.Storage.GetValues("plan_apply"), twin.Storage.GetValues("plan_apply")) {
			t.Error("pinning the noise changed nothing, so the comparison proves little")
		}
	})

	t.Run("past the agents' defaults, depth and rollout length follow the horizon, as in the macro", func(t *testing.T) {
		chain := readFile(t, "testdata/planning/chain_macro.yaml")
		chainTwin := readFile(t, "testdata/planning/chain_twin.yaml")
		macro, twin := runPair(t, chain, chainTwin)
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
		// The controls: the agents' own defaults plan differently here, so the
		// comparison above sees the defaults.
		for _, settings := range []string{"max_tree_depth: 14", "rollout_max_steps: 220"} {
			field, _, _ := strings.Cut(settings, ":")
			iteration := map[string]string{"max_tree_depth": "{type: mcts_tree, environment: *env",
				"rollout_max_steps": "{type: mcts_rollout, environment: *env"}[field]
			control, err := RunToStorage(writeConfig(t, strings.Replace(chainTwin, iteration, iteration+", "+settings, 1)))
			if err != nil {
				t.Fatal(err)
			}
			if equalRows(control.Storage.GetValues("plan_search"), twin.Storage.GetValues("plan_search")) {
				t.Errorf("%s searched alike, so the defaults are not what the comparison checks", settings)
			}
		}
		// And set explicitly, the settings carry over.
		macro, twin = runPair(t,
			strings.Replace(chain, "  pinned_noise: true\n", "  pinned_noise: true\n  max_tree_depth: 3\n  rollout_max_steps: 50\n", 1),
			strings.NewReplacer(
				"{type: mcts_tree, environment: *env", "{type: mcts_tree, environment: *env, max_tree_depth: 3",
				"{type: mcts_rollout, environment: *env", "{type: mcts_rollout, environment: *env, rollout_max_steps: 50",
			).Replace(chainTwin))
		assertStoragesEqual(t, "explicit settings, twin vs macro", twin.Storage, macro.Storage)
	})

	t.Run("a search scores its return normalised against return_range", func(t *testing.T) {
		// One action, worth a reward of 1, over a one-step horizon: every
		// rollout returns 1, which return_range [0, 4] puts at 0.25.
		result, err := RunToStorage(writeConfig(t, `macros:
- type: mcts_planning
  name: plan
  steps: 1
  horizon: 1
  sims_per_decision: 10
  seed: 1
  pinned_noise: true
  return_range: [0, 4]
  actions: [[1]]
  action_partition: gain
  action_param: stride
  reward_partition: gain
  model:
    partitions:
    - {name: gain, iteration: {type: expression, fields: [{name: g}], outputs: ["stride"]}, params: {stride: [0.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
`))
		if err != nil {
			t.Fatal(err)
		}
		// The search row starts with the tree's: the environment's state is
		// [step, return, gain], three wide, and there is one action.
		row := result.Storage.GetValues("plan_search")[1]
		visits, wins := row[agents.MCTSTreeRowVisitsOffset(3)], row[agents.MCTSTreeRowWinsOffset(3, 1)]
		if visits == 0 || wins/visits != 0.25 {
			t.Errorf("the root action scored %v over %v visits, want 0.25 each", wins, visits)
		}
	})

	t.Run("a progress proxy scores rollouts cut short of the horizon", func(t *testing.T) {
		truncated := strings.Replace(readFile(t, "testdata/planning/chain_twin.yaml"),
			"{type: mcts_rollout, environment: *env", "{type: mcts_rollout, environment: *env, rollout_max_steps: 50", 1)
		with, err := RunToStorage(writeConfig(t, truncated))
		if err != nil {
			t.Fatal(err)
		}
		without, err := RunToStorage(writeConfig(t, strings.Replace(truncated, "        progress_partition: progress\n", "", 1)))
		if err != nil {
			t.Fatal(err)
		}
		if equalRows(with.Storage.GetValues("plan_search"), without.Storage.GetValues("plan_search")) {
			t.Error("the search ignored its progress proxy")
		}
	})

	t.Run("exploration may be written as an integer", func(t *testing.T) {
		if _, err := LoadConfig(writeConfigPath(t, strings.Replace(planningTwin,
			"exploration: 0.8", "exploration: 1", 1))); err != nil {
			t.Error(err)
		}
	})

	t.Run("an environment with no model partitions is a config error at load", func(t *testing.T) {
		start := strings.Index(planningTwin, "        model:\n")
		end := strings.Index(planningTwin, "    params_as_partitions: {best_idx_partition")
		yaml := planningTwin[:start] + "        model: {partitions: []}\n" + planningTwin[end:]
		_, err := LoadConfig(writeConfigPath(t, yaml))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "environment simulation needs model.partitions") {
			t.Errorf("got %v", err)
		}
	})

	cases := []struct{ name, from, to, want string }{
		{"an unknown environment type", "        type: simulation\n", "        type: casino\n",
			`mcts_apply: field "environment": unknown environment type "casino" (expected simulation, or a registered environment: `},
		{"an environment reading recorded samples without naming the input", "samples: [[2.0], [8.0]]",
			"samples_from: {partition_name: post}",
			"parameters.samples_from and weights_from read an input's recorded rows when the run starts, so name it"},
		{"an environment field it does not know", "        horizon: 8\n", "        horizon: 8\n        steps: 8\n",
			"environment simulation: yaml: unmarshal errors"},
		{"an environment with no horizon", "        horizon: 8\n", "        horizon: 0\n",
			"environment simulation needs horizon: > 0"},
		{"an environment rewarding a partition its model lacks", "        reward_partition: revenue\n",
			"        reward_partition: profit\n", `reward_partition: no model partition named "profit"`},
		{"a model partition with an unknown iteration", "              type: expression\n              fields: [{name: soc}",
			"              type: battery_model\n              fields: [{name: soc}", `unknown data-spec type "battery_model"`},
		{"an environment with no reward", "        reward_partition: revenue\n", "",
			"environment simulation needs reward_partition: naming the model partition"},
		{"a progress proxy its model lacks", "progress_partition: battery", "progress_partition: charge",
			`progress_partition: no model partition named "charge"`},
		{"parameters with no targets", "targets: [{partition: price, param: scale, indices: [0]}]", "targets: []",
			"parameters needs targets:"},
		{"a parameter target its model lacks", "targets: [{partition: price,", "targets: [{partition: cost,",
			`parameters.targets: no model partition named "cost"`},
		{"a belief observing a partition its model lacks", "observation_partition: price", "observation_partition: meter",
			`parameters.belief.observation_partition: no model partition named "meter"`},
		{"a belief with no observation noise", "variance: 4.0}", "variance: 0}",
			"environment simulation parameters.belief needs variance: > 0"},
		{"a setting a tree does not read", "exploration: 0.8,", "simulations: 400, exploration: 0.8,",
			`mcts_tree: unknown field "simulations"`},
		{"a non-numeric exploration", "exploration: 0.8", "exploration: wide",
			`mcts_tree: field "exploration" must be a number, got string`},
		{"a fractional tree depth", "max_tree_depth: 5", "max_tree_depth: 5.5",
			`mcts_tree: field "max_tree_depth" must be an integer, got float64`},
		{"a non-bool pinned_noise", "max_tree_depth: 5}", "max_tree_depth: 5, pinned_noise: sometimes}",
			`mcts_tree: field "pinned_noise" must be a bool, got string`},
		{"a fractional rollout length", "rollout_max_steps: 4", "rollout_max_steps: 4.5",
			`mcts_rollout: field "rollout_max_steps" must be an integer, got float64`},
		{"a non-integer best index slot", "      type: mcts_apply\n", "      type: mcts_apply\n      best_index_slot: first\n",
			`mcts_apply: field "best_index_slot" must be an integer, got string`},
		{"a rollout with no environment", "{type: mcts_rollout, environment: *env, ", "{type: mcts_rollout, ",
			`mcts_rollout: missing required field "environment"`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			yaml := strings.Replace(planningTwin, c.from, c.to, 1)
			if yaml == planningTwin {
				t.Fatalf("the case did not change the config: %q", c.from)
			}
			_, err := LoadConfig(writeConfigPath(t, yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}

func equalRows(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}
