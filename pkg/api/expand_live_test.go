package api

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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
		return resolveMCTSEnvironment(simulator.ComponentSpec{Type: "simulation", Fields: fields}, newRunScope())
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

// smcConfig is cfg/example_smc_config.yaml with the given replacements.
func smcConfig(t *testing.T, replacements ...string) string {
	t.Helper()
	return strings.NewReplacer(replacements...).Replace(readFile(t, "../../cfg/example_smc_config.yaml"))
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
		"../../cfg/example_smc_config.yaml",
		"testdata/replicated/smc_macro.yaml",
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

	t.Run("SMC runs as its hand-written twin does", func(t *testing.T) {
		written, err := RunToStorage(writeConfig(t, readFile(t, "testdata/replicated/smc_twin.yaml")))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "twin", expandedRun(t, readFile(t, "testdata/replicated/smc_macro.yaml")).Storage,
			written.Storage)
	})

	t.Run("the expanded SMC recovers the observed stream's mean", func(t *testing.T) {
		// The observed stream is drawn around a mean of 2.
		posterior := expandedRun(t, readFile(t, "../../cfg/example_smc_config.yaml")).
			Storage.GetValues("smc_posterior")
		if got := posterior[len(posterior)-1][0]; math.Abs(got-2.0) > 0.5 {
			t.Errorf("posterior mean %v, want ~2", got)
		}
	})

	t.Run("SMC over a file input on its own clock", func(t *testing.T) {
		csv := filepath.Join(t.TempDir(), "obs.csv")
		rows := []string{}
		for i := 0; i < 30; i++ {
			rows = append(rows, fmt.Sprintf("%g,%g", 5+0.5*float64(i), 2+math.Sin(float64(i))))
		}
		if err := os.WriteFile(csv, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// The prediction drifts with time, so the particles' clock must start
		// where the input does.
		config := smcConfig(t, `      iteration: {type: param_values}
      params: {param_values: [0.0]}`, `      iteration: {type: expression, fields: [{name: p}], outputs: ["param_values[0] + 0.1 * (t + dt - 5)"]}
      params: {param_values: [0.0]}`)
		expandedAlike(t, fmt.Sprintf("inputs:\n  data:\n    source: {csv: {path: %q, time_column: 0, state_columns: {obs: [1]}}}\n", csv)+
			config[strings.Index(config, "macros:"):])
	})

	t.Run("SMC observing a partition whose name YAML 1.1 reads as a boolean", func(t *testing.T) {
		// The name is written into the replay's fields, which load untyped, so
		// it must be written quoted to load back as the name y.
		expandedAlike(t, smcConfig(t, "      - name: obs\n", "      - name: \"y\"\n",
			"ref: {partition_name: obs}", "ref: {partition_name: \"y\"}"))
	})

	t.Run("SMC over two parameters, then an analysis of its posterior", func(t *testing.T) {
		expandedAlike(t, smcConfig(t,
			"priors: [{type: uniform, lo: -5.0, hi: 10.0}]", "priors: [{type: uniform, lo: -5.0, hi: 10.0}, {type: uniform, lo: 0.1, hi: 2.0}]",
			"param_names: [mean]", "param_names: [mean, variance]",
			"num_particles: 100", "num_particles: 20",
			`"pred/param_values": [0]`, `"pred/param_values": [0]`+"\n      \"loglike/variance\": [1]",
		)+"- {type: vector_mean, name: m, data: {partition_name: smc_posterior}, window: 2}\n")
	})

	t.Run("SMC over a model stating its own simulation, with no inputs, verbosely", func(t *testing.T) {
		expandedAlike(t, `macros:
- type: smc_inference
  proposal_name: p
  sim_name: s
  posterior_name: q
  num_particles: 4
  num_rounds: 2
  seed: 3
  verbose: true
  timestep: 0.5
  priors: [{type: uniform, lo: 0.0, hi: 4.0}]
  param_names: [mean]
  model:
    simulation:
      output_condition: {type: nil}
      output_function: {type: nil}
      termination_condition: {type: number_of_steps, max_steps: 6}
      timestep_function: {type: constant, stepsize: 1.0}
      init_time_value: 1.0
    partitions:
    - {name: target, iteration: {type: expression, fields: [{name: level}], outputs: ["2 + 0.1 * t"]}, init_state_values: [2.0], state_history_depth: 2}
    - {name: pred, iteration: {type: param_values}, params: {param_values: [0.0]}, init_state_values: [0.0], state_history_depth: 2}
    - name: loglike
      iteration: {type: data_comparison, likelihood: {type: normal}}
      params: {mean: [0.0], variance: [0.5], latest_data_values: [2.0], cumulative: [1], burn_in_steps: [0]}
      params_from_upstream: {mean: {upstream: pred}, latest_data_values: {upstream: target}}
      init_state_values: [0.0]
      state_history_depth: 2
    loglike_partition: loglike
    param_forwarding: {"pred/param_values": [0]}
`)
	})

	t.Run("calibrate, then plan expands exactly, as a run: input the planner reads", func(t *testing.T) {
		config := calibration + strings.Replace(newsvendorPlan, "PARAMETERS",
			"samples_from: {partition_name: post_sampler, burn_in: 100}", 1)
		expanded := expandedAlike(t, config)
		run, ok := expanded.Inputs[calibrationInput]
		if !ok || run.Run == nil || len(expanded.Inputs) != 1 {
			t.Fatalf("the expansion's inputs are %v, want one run: input, %s", expanded.Inputs, calibrationInput)
		}
		// The independent reference: the calibration run alone as a run: input,
		// with a hand-stated planner reading it (TestPlanningParametersFromInputs).
		written, err := RunToStorage(writeConfig(t, "inputs:\n  calibration:\n    run:\n"+indent(calibration, 6)+
			corePlanner(t, "samples_from: {input: calibration, partition_name: post_sampler, burn_in: 100}")))
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunToStorage(expanded)
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "hand-stated", result.Storage, written.Storage)
		rows := result.Storage.GetValues("plan_apply")
		if ordered := rows[len(rows)-1][2]; ordered != 3 {
			t.Errorf("ordered %v, want 3 for the inferred demand", ordered)
		}
	})

	t.Run("two analyses, then a plan over the second's output, then an analysis of the plan", func(t *testing.T) {
		expandedAlike(t, aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: stream}, window: 3}\n"+
			"- {type: vector_mean, name: mm, data: {partition_name: m, value_indices: [0]}, window: 2}\n")+
			strings.Replace(newsvendorPlan, "PARAMETERS", "samples_from: {partition_name: mm, burn_in: 10}", 1)+
			"- {type: vector_mean, name: p, data: {partition_name: plan_apply}, window: 2}\n")
	})

	t.Run("a plan over samples and weights read straight from the inputs", func(t *testing.T) {
		expandedAlike(t, `inputs:
  data:
    simulation:
      steps: 11
      timestep: 1.0
      partitions:
      - name: draws
        iteration: {type: expression, fields: [{name: d}], outputs: ["where(step - 4 * floor(step / 4) < 3, 1, 9)"]}
        init_state_values: [1.0]
        state_history_depth: 1
        seed: 0
      - {name: credence, iteration: {type: constant_values}, init_state_values: [5.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0], state_history_depth: 1, seed: 0}
macros:
`+strings.Replace(newsvendorPlan, "PARAMETERS", "samples_from: {partition_name: draws, burn_in: 2}, weights_from: {partition_name: credence}", 1))
	})

	t.Run("an analysis, then SMC observing its output (calibrate, then infer)", func(t *testing.T) {
		expandedAlike(t, strings.NewReplacer(
			"macros:\n", "macros:\n- {type: vector_mean, name: m, data: {partition_name: obs}, window: 2}\n",
			"ref: {partition_name: obs}", "ref: {partition_name: m}",
			"num_particles: 100", "num_particles: 20").Replace(smcConfig(t)))
	})

	t.Run("an analysis, then SMC observing an input", func(t *testing.T) {
		expandedAlike(t, strings.NewReplacer(
			"macros:\n", "macros:\n- {type: vector_mean, name: m, data: {partition_name: obs}, window: 2}\n",
			"num_particles: 100", "num_particles: 20").Replace(smcConfig(t)))
	})

	cases := []struct{ name, yaml, want string }{
		{"an evolution strategy whose window reads data",
			esConfig(t, "    partitions:\n", "    data: [{partition_name: x}]\n    partitions:\n"),
			"evolution_strategy_optimisation's window reads data:"},
		{"a planner with a belief over an earlier macro's samples",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: stream}, window: 3}\n") +
				strings.Replace(planningMacro(t),
					"    samples: [[2.0], [8.0]]\n    weights: [0.3, 0.7]\n",
					"    samples_from: {partition_name: m}\n", 1),
			"mcts_planning's parameters.belief over recorded samples does not expand"},
		{"a planner sampling a partition nothing supplies",
			strings.NewReplacer("    samples: [[2.0], [8.0]]\n    weights: [0.3, 0.7]\n", "    samples_from: {partition_name: post}\n",
				"    belief: {observation_partition: price, variance: 4.0}\n", "").Replace(
				readFile(t, "testdata/planning/planning_macro.yaml")),
			`mcts_planning's parameters.samples_from reads "post", which no input or earlier macro supplies`},
		{"a planner weighting by a partition nothing supplies",
			calibration + strings.Replace(newsvendorPlan, "PARAMETERS",
				"samples_from: {partition_name: post_sampler}, weights_from: {partition_name: credence}", 1),
			`mcts_planning's parameters.weights_from reads "credence", which no input or earlier macro supplies`},
		{"a calibration whose analysis reads a partition nothing makes",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: gone}, window: 3}\n") +
				strings.Replace(newsvendorPlan, "PARAMETERS", "samples_from: {partition_name: m}", 1),
			`no partition "gone" to read`},
		{"a planner weighting by recorded rows beside inline samples",
			calibration + strings.Replace(newsvendorPlan, "PARAMETERS",
				"samples: [[1.0]], weights_from: {partition_name: post_sampler}", 1),
			"mcts_planning's parameters.weights_from beside inline samples does not expand"},
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
			"macros[0] (mcts_self_play) runs its own clock beside the config's inputs, but reads " +
				"none of them; one run has one clock, so it does not expand"},
		{"a macro with its own clock after an analysis, reading nothing",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: stream}, window: 3}\n" +
				"- {type: mcts_self_play, name: ttt, steps: 2, env: {type: tictactoe}}\n"),
			"macros[1] (mcts_self_play) runs its own clock beside the config's inputs, but reads none of them"},
		{"two macros with their own clocks",
			"macros:\n- {type: mcts_self_play, name: a, steps: 2, env: {type: tictactoe}}\n" +
				"- {type: mcts_self_play, name: b, steps: 2, env: {type: tictactoe}}\n",
			"macros[0] (mcts_self_play) and macros[1] (mcts_self_play) each run their own clock; " +
				"one run has one clock, so a second macro with its own does not expand"},
		{"SMC observing data, with no inputs",
			"macros:\n" + smcConfig(t)[strings.Index(smcConfig(t), "- type: smc_inference"):],
			`smc_inference model.observed_data reads "obs", which no input supplies`},
		{"SMC over a model stating its own simulation, beside inputs it does not read",
			strings.Replace(smcConfig(t), "    observed_data: {name: observed_data, ref: {partition_name: obs}}\n",
				"    simulation: {output_condition: {type: nil}, output_function: {type: nil}, "+
					"termination_condition: {type: number_of_steps, max_steps: 3}, timestep_function: {type: constant, stepsize: 1.0}}\n", 1),
			"macros[0] (smc_inference) runs its own clock beside the config's inputs"},
		{"an analysis after SMC of an input it does not replay",
			smcConfig(t) + "- {type: vector_mean, name: m, data: {partition_name: obs}, window: 2}\n",
			`no partition "obs" to read`},
		{"SMC with an unknown prior",
			smcConfig(t, "{type: uniform, lo: -5.0, hi: 10.0}", "{type: cauchy}"),
			"smc_inference prior 0:"},
		{"SMC over a model whose simulation is not all data",
			"macros:\n- {type: smc_inference, proposal_name: p, sim_name: s, posterior_name: q, num_particles: 2, num_rounds: 1, " +
				"priors: [{type: uniform, lo: 0.0, hi: 1.0}], param_names: [m], model: {partitions: [], loglike_partition: l}}\n",
			"smc_inference model.simulation must be fully data specs (or set observed_data)"},
		{"SMC over a model partition with no iteration",
			smcConfig(t, `    - name: "pred"
      iteration: {type: param_values}
`, `    - name: "pred"
`),
			`partition "pred" needs an iteration: {type: ...}`},
		{"an analysis before the macro with the clock",
			"macros:\n- {type: vector_mean, name: m, data: {partition_name: ttt_apply}, window: 2}\n" +
				"- {type: mcts_self_play, name: ttt, steps: 2, env: {type: tictactoe}}\n",
			"expanding needs a clock: the inputs the macros read, main.simulation, or a first macro that runs its own"},
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
