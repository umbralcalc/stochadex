package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// walkRun is a config of its own: a counter and its running total, 5 steps
// at uneven times.
const walkRun = `main:
  partitions:
  - {name: count, iteration: {type: expression, fields: [{name: c}], outputs: ["c + 1"]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - name: "y"
    iteration: {type: expression, fields: [{name: s}], outputs: ["s + v[0]"]}
    params: {v: [0.0]}
    params_from_upstream: {v: {upstream: count}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 0.5}
    init_time_value: 1.0
`

// replaying is a config replaying a run: input's partitions on its clock.
func replaying(run string) string {
	return "inputs:\n  walk:\n    run:\n" + indent(run, 6) + `main:
  partitions:
  - {name: count, iteration: {type: from_input, input: walk}, state_history_depth: 1, seed: 0}
  - {name: total, iteration: {type: from_input, input: walk, partition: "y"}, state_history_depth: 1, seed: 0}
  simulation:
    termination_condition: {type: input_exhausted, input: walk}
    timestep_function: {type: from_input, input: walk}
    init_time_value: 1.0
`
}

func TestRunInputs(t *testing.T) {
	t.Run("a run: input's rows and times are its config's run alone", func(t *testing.T) {
		alone, err := RunToStorage(writeConfig(t, walkRun))
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := RunToStorage(writeConfig(t, replaying(walkRun)))
		if err != nil {
			t.Fatal(err)
		}
		// The partition named y keeps its name through the input's own text.
		if got, want := fmt.Sprint(replayed.Storage.GetValues("total")), fmt.Sprint(alone.Storage.GetValues("y")); got != want {
			t.Errorf("replayed %s, want %s", got, want)
		}
		if got, want := fmt.Sprint(replayed.Storage.GetValues("count")), fmt.Sprint(alone.Storage.GetValues("count")); got != want {
			t.Errorf("replayed %s, want %s", got, want)
		}
		if got, want := fmt.Sprint(replayed.Storage.GetTimes()), fmt.Sprint(alone.Storage.GetTimes()); got != want {
			t.Errorf("times %s, want %s", got, want)
		}
		// By hand: the total of 1..5 at times 1.5..3.5.
		if got := fmt.Sprint(replayed.Storage.GetValues("total")); got != "[[0] [1] [3] [6] [10] [15]]" {
			t.Errorf("total %s, want the running sum of 1..5", got)
		}
	})

	t.Run("a run: input beside an input of another kind", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, strings.NewReplacer(
			"inputs:\n", "inputs:\n  counts: {source: {inline: {times: [1.0, 1.5, 2.0, 2.5, 3.0, 3.5], partitions: {count: [[7], [7], [7], [7], [7], [7]]}}}}\n",
			"{name: count, iteration: {type: from_input, input: walk}", "{name: count, iteration: {type: from_input, input: counts}",
		).Replace(replaying(walkRun))))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(result.Storage.GetValues("count")); got != "[[7] [7] [7] [7] [7] [7]]" {
			t.Errorf("count %s, want the inline input's 7s", got)
		}
		if got := fmt.Sprint(result.Storage.GetValues("total")); got != "[[0] [1] [3] [6] [10] [15]]" {
			t.Errorf("total %s, want the run: input's running sum", got)
		}
	})

	t.Run("a run: input that is itself a macros config, with a run: input of its own", func(t *testing.T) {
		macros := "inputs:\n  inner:\n    run:\n" + indent(walkRun, 6) +
			"macros:\n- {type: vector_mean, name: count_mean, data: {partition_name: count}, window: 2}\n"
		alone, err := RunToStorage(writeConfig(t, macros))
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := RunToStorage(writeConfig(t, strings.Replace(replaying(macros),
			`{name: total, iteration: {type: from_input, input: walk, partition: "y"}`,
			`{name: total, iteration: {type: from_input, input: walk, partition: count_mean}`, 1)))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := fmt.Sprint(replayed.Storage.GetValues("total")), fmt.Sprint(alone.Storage.GetValues("count_mean")); got != want {
			t.Errorf("replayed %s, want %s", got, want)
		}
	})

	t.Run("a run: input reading a file input of its own", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "obs.csv"), []byte("0,1\n1,2\n2,4\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		inner := fmt.Sprintf("inputs:\n  obs: {source: {csv: {path: %q, time_column: 0, state_columns: {x: [1]}}}}\n",
			filepath.Join(dir, "obs.csv")) +
			"macros:\n- {type: vector_mean, name: m, data: {partition_name: x}, window: 2}\n"
		path := filepath.Join(dir, "outer.yaml")
		outer := "inputs:\n  walk:\n    run:\n" + indent(inner, 6) + `main:
  partitions:
  - {name: total, iteration: {type: from_input, input: walk, partition: m}, state_history_depth: 1, seed: 0}
  simulation:
    termination_condition: {type: input_exhausted, input: walk}
    timestep_function: {type: from_input, input: walk}
    init_time_value: 0.0
`
		if err := os.WriteFile(path, []byte(outer), 0o644); err != nil {
			t.Fatal(err)
		}
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		alone, err := RunToStorage(writeConfig(t, inner))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := fmt.Sprint(result.Storage.GetValues("total")), fmt.Sprint(alone.Storage.GetValues("m")); got != want {
			t.Errorf("total %s, want %s", got, want)
		}
	})

	t.Run("a run: input that fails when it runs fails the run, as it would alone", func(t *testing.T) {
		_, err := RunToStorage(writeConfig(t, strings.Replace(replaying(
			"inputs:\n  obs: {source: {csv: {path: gone.csv, time_column: 0, state_columns: {x: [1]}}}}\n"+
				"macros:\n- {type: vector_mean, name: m, data: {partition_name: x}, window: 2}\n"),
			`partition: "y"`, "partition: m", 1)))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), `loading input "walk"`) {
			t.Errorf("got %v", err)
		}
	})

	for _, c := range []struct{ name, yaml, want string }{
		{"a run: input whose config does not load", replaying(strings.Replace(walkRun, "type: constant,", "type: constnat,", 1)),
			`input "walk"'s run:`},
		{"a run: input with a key nothing reads", replaying(strings.Replace(walkRun, "  simulation:\n", "  simulaton: {}\n  simulation:\n", 1)),
			"simulaton"},
		{"a run: input run as an ensemble", replaying(walkRun + "run: {mode: ensemble, seeds: [1, 2]}\n"),
			`input "walk": a run: input runs once, so its run: mode must be batch, not "ensemble"`},
		{"an input that is both a run and a source", strings.Replace(replaying(walkRun), "    run:\n",
			"    source: {inline: {times: [0.0], partitions: {count: [[0.0]]}}}\n    run:\n", 1),
			`input "walk" needs exactly one of source:, simulation:, run:, stream: or file:`},
	} {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}

// calibration infers the mean of a stream (truly 3) with posterior_estimation;
// post_sampler's rows are draws from its running posterior.
const calibration = `data:
  steps: 300
  timestep: 1.0
  partitions:
  - name: observations
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [3.0], covariance_matrix: [0.25]}
    init_state_values: [3.0]
    state_history_depth: 20
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
      params: {default_covariance: [4.0], cov_burn_in_steps: [50]}
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
      depth: 20
    window_data_history_depth: {observations: 20}
  past_discount: 0.999
  memory_depth: 20
  seed: 1234
`

// newsvendorPlan plans an order (1, 3 or 9) against a demand drawn from the
// parameter samples; its parameters are given by PARAMETERS.
const newsvendorPlan = `- type: mcts_planning
  name: plan
  steps: 2
  horizon: 1
  sims_per_decision: 200
  seed: 3
  return_range: [-200, 200]
  actions: [[1], [3], [9]]
  action_partition: order
  action_param: param_values
  reward_partition: profit
  parameters: {PARAMETERS, targets: [{partition: profit, param: demand, indices: [0]}]}
  model:
    partitions:
    - {name: order, iteration: {type: param_values}, params: {param_values: [0.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
    - name: profit
      iteration: {type: expression, fields: [{name: p}], outputs: ["10 * min(quantity, demand) - 6 * quantity"]}
      params: {quantity: [0.0], demand: [0.0]}
      params_from_upstream: {quantity: {upstream: order}}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
`

// corePlanner writes newsvendorPlan in the core language with the given
// parameters, through the expansion of the macro with a placeholder sample.
func corePlanner(t *testing.T, parameters string) string {
	t.Helper()
	config, err := LoadConfig(writeConfigPath(t, "macros:\n"+
		strings.Replace(newsvendorPlan, "PARAMETERS", "samples: [[1.0]]", 1)))
	if err != nil {
		t.Fatal(err)
	}
	written, err := ExpandToYAML(config)
	if err != nil {
		t.Fatal(err)
	}
	// The environment is stated by the apply, tree and rollout partitions.
	if n := strings.Count(string(written), "samples: [[1.0]]"); n != 3 {
		t.Fatalf("the placeholder sample appears %d times, want 3:\n%s", n, written)
	}
	return strings.ReplaceAll(string(written), "samples: [[1.0]]", parameters)
}

func TestPlanningParametersFromInputs(t *testing.T) {
	plan := func(t *testing.T, yaml string) [][]float64 {
		t.Helper()
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		return result.Storage.GetValues("plan_apply")
	}

	t.Run("calibrate, then plan: a run: input's samples plan as the macro chain does", func(t *testing.T) {
		chain := plan(t, calibration+strings.Replace(newsvendorPlan, "PARAMETERS",
			"samples_from: {partition_name: post_sampler, burn_in: 100}", 1))
		core := plan(t, "inputs:\n  calibration:\n    run:\n"+indent(calibration, 6)+corePlanner(t,
			"samples_from: {input: calibration, partition_name: post_sampler, burn_in: 100}"))
		if fmt.Sprint(core) != fmt.Sprint(chain) {
			t.Errorf("core plan %v, want the macro chain's %v", core, chain)
		}
		// Demand is truly 3, so ordering 3 is right once the posterior is.
		if ordered := core[len(core)-1][2]; ordered != 3 {
			t.Errorf("ordered %v, want 3", ordered)
		}
	})

	draws := `inputs:
  data:
    simulation:
      steps: 11
      timestep: 1.0
      partitions:
      - name: draws
        iteration: {type: expression, fields: [{name: d}, {name: e}], outputs: ["where(step - 4 * floor(step / 4) < 3, 1, 9)", "step"]}
        init_state_values: [1.0, 0.0]
        state_history_depth: 1
        seed: 0
      - name: credence
        iteration: {type: constant_values}
        init_state_values: [5.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0]
        state_history_depth: 1
        seed: 0
`
	t.Run("samples and weights read straight from an input plan as the macro does", func(t *testing.T) {
		macro := plan(t, draws+"macros:\n"+strings.Replace(newsvendorPlan, "PARAMETERS",
			"samples_from: {partition_name: draws, value_indices: [0], burn_in: 0}, weights_from: {partition_name: credence}", 1))
		core := plan(t, draws+corePlanner(t,
			"samples_from: {input: data, partition_name: draws, value_indices: [0]}, weights_from: {input: data, partition_name: credence}"))
		if fmt.Sprint(core) != fmt.Sprint(macro) {
			t.Errorf("core plan %v, want the macro's %v", core, macro)
		}
		// The weights decide the plan: with them uniform, it differs.
		uniform := plan(t, draws+corePlanner(t,
			"samples_from: {input: data, partition_name: draws, value_indices: [0]}"))
		if fmt.Sprint(uniform) == fmt.Sprint(core) {
			t.Error("the weights changed nothing, so the comparison proves little")
		}
	})

	t.Run("samples and weights from inputs of their own", func(t *testing.T) {
		// draws's partitions split across two inputs on one clock.
		split := strings.Replace(draws, "      - name: credence\n",
			"  beliefs:\n    simulation:\n      steps: 11\n      timestep: 1.0\n      partitions:\n      - name: credence\n", 1)
		macro := plan(t, split+"macros:\n"+strings.Replace(newsvendorPlan, "PARAMETERS",
			"samples_from: {partition_name: draws, value_indices: [0], burn_in: 0}, weights_from: {partition_name: credence}", 1))
		core := plan(t, split+corePlanner(t,
			"samples_from: {input: data, partition_name: draws, value_indices: [0]}, weights_from: {input: beliefs, partition_name: credence}"))
		if fmt.Sprint(core) != fmt.Sprint(macro) {
			t.Errorf("core plan %v, want the macro's %v", core, macro)
		}
	})

	t.Run("each ensemble member binds its own", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, draws+corePlanner(t,
			"samples_from: {input: data, partition_name: draws, value_indices: [0]}")+"run: {mode: ensemble, seeds: [1, 2]}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Members) != 2 {
			t.Fatalf("got %d members", len(result.Members))
		}
	})

	for _, c := range []struct {
		name, parameters string
		kind             ErrorKind
		want             string
	}{
		{"a partition the input lacks", "samples_from: {input: data, partition_name: drawz}",
			ErrData, `input "data" has no partition "drawz" (it has: credence, draws)`},
		{"a weights partition the input lacks", "samples_from: {input: data, partition_name: draws, value_indices: [0]}, weights_from: {input: data, partition_name: credense}",
			ErrData, `input "data" has no partition "credense" (it has: credence, draws)`},
		{"a burn-in past every row", "samples_from: {input: data, partition_name: draws, burn_in: 50}",
			ErrData, "burn_in 50 discards all 12 recorded rows"},
		{"weights that do not match the samples", "samples_from: {input: data, partition_name: draws, value_indices: [0], burn_in: 2}, weights_from: {input: data, partition_name: credence}",
			ErrData, "supplies 12 weights for 10 samples"},
		{"weights that give no sample credence", "samples_from: {input: data, partition_name: draws, value_indices: [0]}, weights: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]",
			ErrData, "binding a planning environment's parameters: agents: parameter weights sum to zero"},
		{"an input that cannot be read", "samples_from: {input: gone, partition_name: draws}, weights_from: {input: data, partition_name: credence}",
			ErrUnavailable, `loading input "gone"`},
		{"a weights input that cannot be read", "samples_from: {input: data, partition_name: draws, value_indices: [0]}, weights_from: {input: gone, partition_name: credence}",
			ErrUnavailable, `loading input "gone"`},
	} {
		t.Run(c.name+" fails the run, saying why", func(t *testing.T) {
			inputs := draws
			if c.kind == ErrUnavailable {
				inputs += "  gone: {source: {csv: {path: gone.csv, time_column: 0, state_columns: {draws: [1]}}}}\n"
			}
			_, err := RunToStorage(writeConfig(t, inputs+corePlanner(t, c.parameters)))
			if KindOf(err) != c.kind || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected %v containing %q, got %v", c.kind, c.want, err)
			}
		})
	}

	for _, c := range []struct{ name, yaml, want string }{
		{"weights from an input beside inline samples", draws + corePlanner(t,
			"samples: [[1.0]], weights_from: {input: data, partition_name: credence}"),
			`parameters.weights_from reads input "data" when the run starts, so its samples must too`},
		{"samples from an input beside weights by name", draws + corePlanner(t,
			"samples_from: {input: data, partition_name: draws}, weights_from: {partition_name: credence}"),
			"so name it"},
		{"a belief over samples from an input", draws + strings.ReplaceAll(corePlanner(t,
			"samples_from: {input: data, partition_name: draws}"), "targets:", "belief: {observation_partition: profit, variance: 1.0}, targets:"),
			"parameters.belief needs its samples when the config loads"},
		{"samples from an undeclared input", draws + corePlanner(t, "samples_from: {input: calibration, partition_name: draws}"),
			`a planning environment's parameters.samples_from names input "calibration", which inputs: does not declare`},
		{"weights from an undeclared input", draws + corePlanner(t,
			"samples_from: {input: data, partition_name: draws}, weights_from: {input: calibration, partition_name: credence}"),
			`a planning environment's parameters.weights_from names input "calibration", which inputs: does not declare`},
		{"targets missing", draws + strings.ReplaceAll(corePlanner(t, "samples_from: {input: data, partition_name: draws}"),
			", targets: [{partition: profit, param: demand, indices: [0]}]", ""),
			"needs targets:"},
		{"an mcts_planning macro naming an input", draws + "macros:\n" + strings.Replace(newsvendorPlan, "PARAMETERS",
			"samples_from: {input: data, partition_name: draws}", 1),
			"input: is for a {type: simulation} environment"},
	} {
		t.Run(c.name+" is a config error", func(t *testing.T) {
			config, err := LoadConfig(writeConfigPath(t, c.yaml))
			if err == nil {
				_, err = RunToStorage(config)
			}
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
