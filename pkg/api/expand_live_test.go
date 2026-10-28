package api

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// stochasticSelfPlay is a decision problem over a noisy model: an environment
// that samples successors, so an mcts_tree over it searches chance nodes
// unless told not to.
const stochasticSelfPlay = `
type: simulation
horizon: 4
return_range: [-50, 50]
actions: [[1], [-1]]
action_partition: walk
action_param: move
reward_partition: walk
model:
  partitions:
  - name: walk
    iteration:
      type: expression
      fields: [{name: x}]
      outputs: ["x + move + shared(normal(0, 1))"]
    params: {move: [0.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
`

func init() {
	RegisterEnvironment("stochastic_walk", func(simulator.ComponentSpec) (MCTSEnvironment, error) {
		var fields map[string]interface{}
		if err := yaml.Unmarshal([]byte(stochasticSelfPlay), &fields); err != nil {
			return nil, err
		}
		delete(fields, "type")
		return resolveMCTSEnvironment(simulator.ComponentSpec{Type: "simulation", Fields: fields})
	})
}

// expandedRun expands a config and runs the expansion.
func expandedRun(t *testing.T, yaml string) *RunResult {
	t.Helper()
	config, err := LoadConfig(writeConfigPath(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := Expand(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunToStorage(expanded)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// esConfig is cfg/example_evolution_strategy_config.yaml's macro with the
// given replacements.
func esConfig(t *testing.T, replacements ...string) string {
	t.Helper()
	return strings.NewReplacer(replacements...).Replace(
		readFile(t, "../../cfg/example_evolution_strategy_config.yaml"))
}

// planningMacro is testdata/planning/planning_macro.yaml's macro entry alone,
// to follow another.
func planningMacro(t *testing.T) string {
	t.Helper()
	config := readFile(t, "testdata/planning/planning_macro.yaml")
	return config[strings.Index(config, "- type: mcts_planning"):]
}

func TestExpandLiveMacros(t *testing.T) {
	for _, path := range []string{
		"../../cfg/example_evolution_strategy_config.yaml",
		"../../cfg/example_mcts_config.yaml",
		"../../cfg/example_planning_config.yaml",
		"testdata/planning/planning_macro.yaml",
	} {
		t.Run(path+" expands exactly", func(t *testing.T) {
			expandedAlike(t, readFile(t, path))
		})
	}

	t.Run("each search runs as its hand-written twin does", func(t *testing.T) {
		for macro, twin := range map[string]string{
			"../../cfg/example_mcts_config.yaml":     "testdata/selfplay/ttt_twin.yaml",
			"../../cfg/example_planning_config.yaml": "testdata/planning/example_twin.yaml",
			"testdata/planning/planning_macro.yaml":  "testdata/planning/planning_twin.yaml",
		} {
			written, err := RunToStorage(writeConfig(t, readFile(t, twin)))
			if err != nil {
				t.Fatal(err)
			}
			assertStoragesEqual(t, twin, expandedRun(t, readFile(t, macro)).Storage, written.Storage)
		}
	})

	t.Run("the expanded planner banks the example's optimum", func(t *testing.T) {
		// The price cycles 10, 10, 90, 90 and the battery holds one unit, so
		// the best 8 steps buy cheap and sell dear twice: 2 * (90 - 10).
		rows := expandedRun(t, readFile(t, "../../cfg/example_planning_config.yaml")).
			Storage.GetValues("plan_apply")
		if banked := rows[len(rows)-1][1]; banked != 160 {
			t.Errorf("banked %v, want 160", banked)
		}
	})

	t.Run("the expanded evolution strategy finds the reward's optimum", func(t *testing.T) {
		// The reward is the negative squared distance from [3, -2].
		means := expandedRun(t, readFile(t, "../../cfg/example_evolution_strategy_config.yaml")).
			Storage.GetValues("es_mean")
		final := means[len(means)-1]
		if math.Abs(final[0]-3) > 1e-2 || math.Abs(final[1]+2) > 1e-2 {
			t.Errorf("es_mean ended at %v, want [3, -2]", final)
		}
	})

	t.Run("the clock is the macro's steps of its timestep, from zero", func(t *testing.T) {
		times := expandedRun(t, esConfig(t, "steps: 1000\n  timestep: 1.0", "steps: 4\n  timestep: 0.25")).
			Storage.GetTimes()
		if got := fmt.Sprint(times); got != "[0 0.25 0.5 0.75 1]" {
			t.Errorf("times %v, want [0 0.25 0.5 0.75 1]", got)
		}
	})

	t.Run("an evolution strategy with every field and wiring set", func(t *testing.T) {
		expandedAlike(t, esConfig(t,
			"steps: 1000\n  timestep: 1.0", "steps: 40\n  timestep: 0.5",
			"discount_factor: 0.0", "discount_factor: 0.9",
			"sampler: {name: es_sampler, default: [0.0, 0.0]}", "sampler: {name: es_sampler, default: [1.0, -1.0]}",
			// The window's own partition reads the sample, and the reward
			// reads it in turn within the window.
			`        params: {sample_values: [0.0, 0.0]}`,
			`        params: {sample_values: [0.0, 0.0], offset: [0.0]}
        params_from_upstream: {offset: {upstream: sim_partition}}`,
			"(sample_values[1]+2))\"]", "(sample_values[1]+2)) + offset[0]\"]",
			`        iteration: {type: constant_values}
        init_state_values: [0.0]`,
			`        iteration: {type: expression, fields: [{name: s}], outputs: ["0.1 * sample[0]"]}
        params: {sample: [0.0, 0.0]}
        init_state_values: [0.0]`,
			"        seed: 0\n", "        seed: 0\n      outside_upstreams: {sample: {upstream: es_sampler}}\n",
		))
	})

	t.Run("an evolution strategy whose covariance collapses falls back to its default", func(t *testing.T) {
		// At a learning rate of 1 the first update is the covariance of one
		// ranked sample, zero, so the sampler draws from the default instead.
		expandedAlike(t, esConfig(t, "steps: 1000", "steps: 20",
			"[4.0, 0.0, 0.0, 4.0], learning_rate: 0.1", "[2.0, 0.5, 0.5, 3.0], learning_rate: 1.0"))
	})

	t.Run("an evolution strategy, then an analysis of its output", func(t *testing.T) {
		expandedAlike(t, esConfig(t, "steps: 1000", "steps: 30")+
			"- {type: vector_mean, name: m, data: {partition_name: es_mean}, window: 3}\n")
	})

	t.Run("self-play with every search setting", func(t *testing.T) {
		expandedAlike(t, `macros:
- {type: mcts_self_play, name: ttt, steps: 6, sims_per_ply: 40, simulations: 7, exploration: 0.9,
   max_tree_depth: 1, rollout_max_steps: 3, seed: 8, timestep: 2.0, env: {type: tictactoe}}
`)
	})

	t.Run("self-play on the environment's defaults", func(t *testing.T) {
		expandedAlike(t, "macros:\n- {type: mcts_self_play, name: ttt, steps: 3, env: {type: eleven_sims_environment}}\n")
	})

	t.Run("self-play over an environment that samples successors searches no chance nodes", func(t *testing.T) {
		expandedAlike(t, "macros:\n- {type: mcts_self_play, name: w, steps: 5, sims_per_ply: 30, seed: 3, env: {type: stochastic_walk}}\n")
	})

	t.Run("a planner with pinned noise and a timestep", func(t *testing.T) {
		expandedAlike(t, strings.Replace(readFile(t, "testdata/planning/planning_macro.yaml"),
			"  sims_per_decision: 60\n", "  sims_per_decision: 60\n  pinned_noise: true\n  timestep: 0.5\n", 1))
	})

	cases := []struct{ name, yaml, want string }{
		{"an evolution strategy whose window reads data",
			esConfig(t, "    partitions:\n", "    data: [{partition_name: x}]\n    partitions:\n"),
			"evolution_strategy_optimisation's window reads data:"},
		{"a planner sampling an earlier macro's output (calibrate, then plan)",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: stream}, window: 3}\n") +
				strings.Replace(planningMacro(t),
					"    samples: [[2.0], [8.0]]\n    weights: [0.3, 0.7]\n",
					"    samples_from: {partition_name: m}\n", 1),
			"macros[1] (mcts_planning) runs its own clock beside the config's inputs"},
		{"a planner sampling recorded parameters, with no inputs",
			strings.Replace(readFile(t, "testdata/planning/planning_macro.yaml"),
				"    samples: [[2.0], [8.0]]\n    weights: [0.3, 0.7]\n",
				"    samples_from: {partition_name: post}\n", 1),
			"parameters.samples_from and weights_from read recorded rows before the run"},
		{"a planner with no name",
			strings.Replace(readFile(t, "testdata/planning/planning_macro.yaml"), "  name: plan\n", "", 1),
			"mcts_planning needs a name"},
		{"a planner with no steps",
			strings.Replace(readFile(t, "testdata/planning/planning_macro.yaml"), "  steps: 8\n", "", 1),
			"mcts_planning needs steps: > 0"},
		{"self-play with no steps",
			"macros:\n- {type: mcts_self_play, name: ttt, env: {type: tictactoe}}\n",
			"mcts_self_play needs steps: > 0"},
		{"self-play with no env",
			"macros:\n- {type: mcts_self_play, name: ttt, steps: 2}\n",
			"mcts_self_play needs an env:"},
		{"self-play with no name",
			"macros:\n- {type: mcts_self_play, steps: 2, env: {type: tictactoe}}\n",
			"mcts_self_play needs a name:"},
		{"a macro with its own clock beside inputs",
			aggregationConfig("- {type: mcts_self_play, name: ttt, steps: 2, env: {type: tictactoe}}\n"),
			"macros[0] (mcts_self_play) runs its own clock beside the config's inputs, which run " +
				"another; one run has one clock, and a macro with its own beside inputs (calibrate, " +
				"then plan) does not expand yet"},
		{"two macros with their own clocks",
			"macros:\n- {type: mcts_self_play, name: a, steps: 2, env: {type: tictactoe}}\n" +
				"- {type: mcts_self_play, name: b, steps: 2, env: {type: tictactoe}}\n",
			"macros[0] (mcts_self_play) and macros[1] (mcts_self_play) each run their own clock; " +
				"one run has one clock, so a second macro with its own does not expand"},
		{"an analysis before the macro with the clock",
			"macros:\n- {type: vector_mean, name: m, data: {partition_name: ttt_apply}, window: 2}\n" +
				"- {type: mcts_self_play, name: ttt, steps: 2, env: {type: tictactoe}}\n",
			"expanding needs the inputs the macros read, or a first macro that runs its own clock"},
	}
	for _, c := range cases {
		t.Run(c.name+" does not expand, saying why", func(t *testing.T) {
			config, err := LoadConfig(writeConfigPath(t, c.yaml))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Expand(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
