package api

import (
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// namesRun is a main run of a counting source and the partition under test, which
// is spliced in as walker. The source counts 1, 2, 3, ... so a walker reading it has
// a value to read; the clock steps by 0.5 so dt is distinguishable from a count.
func namesRun(walker string) string {
	return `main:
  partitions:
  - name: source
    iteration: {type: expression, fields: [{name: level}], outputs: ["level + 1"]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
` + walker + `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 0.5}
    init_time_value: 0.0
`
}

// assertLoadRefused checks that config fails LoadConfig as a config error whose
// message carries every one of want.
func assertLoadRefused(t *testing.T, config string, want ...string) {
	t.Helper()
	loaded, err := LoadConfig(writeConfigPath(t, config))
	if err == nil {
		t.Fatalf("LoadConfig accepted the config (got %v), want an error containing %q",
			loaded, want)
	}
	if KindOf(err) != ErrConfig {
		t.Errorf("error kind %v, want ErrConfig: %v", KindOf(err), err)
	}
	for _, fragment := range want {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not mention %q", err, fragment)
		}
	}
}

// assertRunRefused checks that config loads but RunToStorage fails as a config
// error carrying every one of want: for the parts of a config, such as a
// sub-simulation or a planner's model, that are built when the run starts.
func assertRunRefused(t *testing.T, config string, want ...string) {
	t.Helper()
	loaded, err := LoadConfig(writeConfigPath(t, config))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	_, err = RunToStorage(loaded)
	if err == nil {
		t.Fatalf("RunToStorage succeeded, want an error containing %q", want)
	}
	if KindOf(err) != ErrConfig {
		t.Errorf("error kind %v, want ErrConfig: %v", KindOf(err), err)
	}
	for _, fragment := range want {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not mention %q", err, fragment)
		}
	}
}

// walkerValues loads and runs config, and returns the walker partition's rows.
func walkerValues(t *testing.T, config string) []float64 {
	t.Helper()
	storage := runLoaded(t, config)
	rows := storage.GetValues("walker")
	values := make([]float64, len(rows))
	for i, row := range rows {
		values[i] = row[0]
	}
	return values
}

// TestExpressionNamesAreRefusedAtLoad pins that an expression partition whose
// names would resolve ambiguously — a declaration spelled like a builtin, or one
// name declared twice — is a config error at load, naming the partition, the
// clash and the rename that fixes it. Each case was silently resolved by the
// order the environment is filled in before.
func TestExpressionNamesAreRefusedAtLoad(t *testing.T) {
	walker := func(iteration, extra string) string {
		return "  - name: walker\n    iteration: " + iteration + "\n" + extra +
			"    init_state_values: [0.0]\n    state_history_depth: 1\n    seed: 0\n"
	}
	cases := []struct {
		name   string
		config string
		want   []string
	}{
		{
			// The planning bug: an action param called step read as the step number.
			name: "a params key spelled like a builtin",
			config: namesRun(walker(
				`{type: expression, fields: [{name: x}], outputs: ["x + step"]}`,
				"    params: {step: [5.0]}\n")),
			want: []string{`expression partition "walker"`, `params key "step"`,
				"builtin step (the step number)", "rename the params key"},
		},
		{
			name: "a field spelled like a builtin, in an expressions: block",
			config: namesRun("  - name: walker\n    init_state_values: [0.0]\n"+
				"    state_history_depth: 1\n    seed: 0\n") +
				"  expressions:\n  - partition: walker\n    fields: [{name: dt}]\n" +
				"    outputs: [\"dt + 1\"]\n",
			want: []string{`expression partition "walker"`, `field "dt"`,
				"builtin dt (the timestep increment)", "rename the field"},
		},
		{
			name: "an upstream alias spelled like a builtin",
			config: namesRun(walker(
				`{type: expression, upstreams: {t: source}, fields: [{name: x}], outputs: ["t[0]"]}`, "")),
			want: []string{`expression partition "walker"`, `upstream alias "t"`,
				"builtin t (the current time)", "rename the upstream alias"},
		},
		{
			name: "a binding spelled like a builtin",
			config: namesRun(walker(`{type: expression, fields: [{name: x}], `+
				`bindings: [{name: pi, expr: "3"}], outputs: ["pi"]}`, "")),
			want: []string{`expression partition "walker"`, `binding "pi"`,
				"builtin pi (the constant pi)", "rename the binding"},
		},
		{
			name: "a params_from_upstream key spelled like a builtin",
			config: namesRun(walker(
				`{type: expression, fields: [{name: x}], outputs: ["x + dt"]}`,
				"    params_from_upstream: {dt: {upstream: source}}\n")),
			want: []string{`expression partition "walker"`, `params key "dt"`, "builtin dt"},
		},
		{
			name: "a params_as_partitions key spelled like a builtin",
			config: namesRun(walker(
				`{type: expression, fields: [{name: x}], outputs: ["x + t"]}`,
				"    params_as_partitions: {t: [source]}\n")),
			want: []string{`expression partition "walker"`, `params key "t"`, "builtin t"},
		},
		{
			name: "a field and a params key with one name",
			config: namesRun(walker(
				`{type: expression, fields: [{name: x}], outputs: ["x + 1"]}`,
				"    params: {x: [5.0]}\n")),
			want: []string{`expression partition "walker"`,
				`"x" is both a field and a params key`, "rename one"},
		},
		{
			name: "a field and an upstream alias with one name",
			config: namesRun(walker(
				`{type: expression, upstreams: {x: source}, fields: [{name: x}], outputs: ["x + 1"]}`, "")),
			want: []string{`"x" is both a field and an upstream alias`},
		},
		{
			name: "an upstream alias and a binding with one name",
			config: namesRun(walker(`{type: expression, upstreams: {up: source}, `+
				`fields: [{name: x}], bindings: [{name: up, expr: "1"}], outputs: ["up"]}`, "")),
			want: []string{`"up" is both an upstream alias and a binding`},
		},
		{
			name: "a params key and a binding with one name",
			config: namesRun(walker(`{type: expression, fields: [{name: x}], `+
				`bindings: [{name: rate, expr: "rate * 2"}], outputs: ["x + rate"]}`,
				"    params: {rate: [1.0]}\n")),
			want: []string{`"rate" is both a params key and a binding`},
		},
		{
			name: "two fields with one name",
			config: namesRun(walker(
				`{type: expression, fields: [{name: x}, {name: x}], outputs: ["x", "x"]}`, "")),
			want: []string{`expression partition "walker"`, `two fields are named "x"`,
				"rename one"},
		},
		{
			name: "two bindings with one name",
			config: namesRun(walker(`{type: expression, fields: [{name: x}], `+
				`bindings: [{name: b, expr: "1"}, {name: b, expr: "b + 1"}], outputs: ["b"]}`, "")),
			want: []string{`two bindings are named "b"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertLoadRefused(t, c.config, c.want...)
		})
	}

	t.Run("positive control: the renamed partition loads and reads both names", func(t *testing.T) {
		// Reference: x gains stride (5) plus dt (0.5) each step.
		got := walkerValues(t, namesRun(walker(
			`{type: expression, fields: [{name: x}], outputs: ["x + stride + dt"]}`,
			"    params: {stride: [5.0]}\n")))
		want := []float64{0, 5.5, 11, 16.5, 22}
		if !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("walker = %v, want %v", got, want)
		}
	})
	t.Run("positive control: an each index may reuse a builtin's name", func(t *testing.T) {
		// A lane index is bound only inside its each, so it shadows nothing a
		// declaration could: each(2, step, step) is [0, 1], whose sum is 1.
		got := walkerValues(t, namesRun(walker(
			`{type: expression, fields: [{name: x}], outputs: ["x + sum(each(2, step, step))"]}`, "")))
		want := []float64{0, 1, 2, 3, 4}
		if !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("walker = %v, want %v", got, want)
		}
	})
	t.Run("positive control: a key both in params and params_from_upstream is one key", func(t *testing.T) {
		// The source counts 1, 2, 3, 4 and walker reads it within the step.
		got := walkerValues(t, namesRun(walker(
			`{type: expression, fields: [{name: x}], outputs: ["x + level"]}`,
			"    params: {level: [0.0]}\n    params_from_upstream: {level: {upstream: source}}\n")))
		want := []float64{0, 1, 3, 6, 10}
		if !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("walker = %v, want %v", got, want)
		}
	})
}

// forwardingRun hosts an embedded run whose expression partition, inner, is
// forwarded the params key named param by its host.
func forwardingRun(param, output string) string {
	return `main:
  partitions:
  - name: host
    params: {burn_in_steps: [0], inner/` + param + `: [3.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: host
  partitions:
  - name: inner
    iteration: {type: expression, fields: [{name: x}], outputs: ["` + output + `"]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 1}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
}

// nestedNamesRun states a run in place whose expression partition holds the
// params key param, and whose host forwards it the key forwarded.
func nestedNamesRun(param, forwarded string) string {
	return `main:
  partitions:
  - name: host
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - name: inner
          iteration: {type: expression, fields: [{name: x}], outputs: ["x + 1"]}
          params: {` + param + `: [1.0]}
          init_state_values: [0.0]
          state_history_depth: 1
          seed: 0
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 1}
          timestep_function: {type: constant, stepsize: 1.0}
          init_time_value: 0.0
    params: {burn_in_steps: [0], inner/` + forwarded + `: [2.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
}

// TestExpressionNamesAreRefusedInsideHostedRuns pins that the check reaches the
// runs a partition hosts, including the params a host forwards, which the inner
// partition's own params never list.
func TestExpressionNamesAreRefusedInsideHostedRuns(t *testing.T) {
	t.Run("a host forwarding a builtin's name into an embedded run", func(t *testing.T) {
		assertLoadRefused(t, forwardingRun("step", "x + step"),
			`embedded run "host": expression partition "inner"`, `params key "step"`,
			"rename the params key")
	})
	t.Run("positive control: forwarding a free name", func(t *testing.T) {
		// Each outer step reruns inner from 0 for one step: 0 + 3.
		storage := runLoaded(t, forwardingRun("gain", "x + gain"))
		rows := storage.GetValues("host")
		if got := rows[len(rows)-1]; !floats.EqualApprox(got, []float64{3}, 1e-12) {
			t.Errorf("host's last row = %v, want [3]", got)
		}
	})
	t.Run("a nested run's expression partition", func(t *testing.T) {
		assertLoadRefused(t, nestedNamesRun("dt", "gain"),
			`partition "host"`, `expression partition "inner"`, `params key "dt"`)
	})
	t.Run("a host forwarding a builtin's name into its nested run", func(t *testing.T) {
		// The nested run's own preflight cannot see this: inner's params do not
		// list the key, its host's do.
		assertLoadRefused(t, nestedNamesRun("rate", "pi"),
			`partition "host"'s nested run: expression partition "inner"`,
			`params key "pi"`)
	})
	t.Run("positive control: a nested run with free names", func(t *testing.T) {
		storage := runLoaded(t, nestedNamesRun("rate", "gain"))
		rows := storage.GetValues("host")
		if got := rows[len(rows)-1]; !floats.EqualApprox(got, []float64{1}, 1e-12) {
			t.Errorf("host's last row = %v, want [1]", got)
		}
	})
}

// subSimulationRun is a macros: config whose input sub-simulation holds an
// expression partition with the given params key.
func subSimulationRun(param string) string {
	return `inputs:
  data:
    simulation:
      steps: 20
      timestep: 1.0
      partitions:
      - {name: x, init_state_values: [0.0], state_history_depth: 1, seed: 0, params: {` + param + `: [1.0]}}
      - {name: y, init_state_values: [0.0], state_history_depth: 1, seed: 0}
      expressions:
      - {partition: x, fields: [{name: v}], outputs: ["t"]}
      - {partition: y, fields: [{name: v}], outputs: ["2 * t + 1"]}
macros:
- type: scalar_regression_stats
  name: ols
  y: {partition_name: y}
  x: {partition_name: x}
  intercept: true
  mode: cumulative
`
}

// planningRun is a small mcts_planning config whose action param is named param.
func planningRun(param string) string {
	return `macros:
- type: mcts_planning
  name: plan
  steps: 2
  horizon: 2
  sims_per_decision: 8
  seed: 99
  return_range: [-10, 10]
  actions: [[1], [0]]
  action_partition: battery
  action_param: ` + param + `
  reward_partition: battery
  model:
    partitions:
    - name: battery
      iteration:
        type: expression
        fields: [{name: soc}]
        outputs: ["clamp(soc + ` + param + `, 0, 1)"]
      params: {` + param + `: [0.0]}
      init_state_values: [0.0]
      state_history_depth: 1
      seed: 0
`
}

// TestExpressionNamesAreRefusedWhenAModelIsBuilt pins the check on the parts of a
// config built as the run starts: a sub-simulation, and an mcts_planning model,
// whose action param — the case that found this — the planner writes into the
// model's params.
func TestExpressionNamesAreRefusedWhenAModelIsBuilt(t *testing.T) {
	t.Run("a sub-simulation's expression partition", func(t *testing.T) {
		assertRunRefused(t, subSimulationRun("t"),
			`sub-simulation expression partition "x"`, `params key "t"`)
	})
	t.Run("positive control: a sub-simulation with free names", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, subSimulationRun("rate")))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunToStorage(config); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an mcts_planning action param spelled like a builtin", func(t *testing.T) {
		assertRunRefused(t, planningRun("step"),
			`mcts_planning model expression partition "battery"`, `params key "step"`,
			"builtin step (the step number)")
	})
	t.Run("an mcts_planning action param the model never declares", func(t *testing.T) {
		// The planner writes the action whether or not the model's params list it.
		config := strings.Replace(planningRun("step"), "      params: {step: [0.0]}\n", "", 1)
		assertRunRefused(t, config, `params key "step"`)
	})
	t.Run("an mcts_planning parameter target clashing with a field", func(t *testing.T) {
		config := planningRun("dispatch") + `  parameters:
    samples: [[0.5]]
    targets: [{partition: battery, param: soc, indices: [0]}]
`
		assertRunRefused(t, config, `"soc" is both a field and a params key`)
	})
	t.Run("positive control: a free action param plans", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, planningRun("dispatch")))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunToStorage(config); err != nil {
			t.Fatal(err)
		}
	})
}

// TestYAMLBooleanNamesAreRefusedAtLoad pins that a name YAML 1.1 reads as a
// boolean — an unquoted y, n, yes, no, on or off — is a load error saying to
// quote it, where before it reached the expression as the name true or false.
func TestYAMLBooleanNamesAreRefusedAtLoad(t *testing.T) {
	walker := func(iteration string) string {
		return "  - name: walker\n    iteration: " + iteration + "\n" +
			"    init_state_values: [0.0]\n    state_history_depth: 1\n    seed: 0\n"
	}
	t.Run("an unquoted field name", func(t *testing.T) {
		assertLoadRefused(t, namesRun(walker(
			`{type: expression, fields: [{name: n}], outputs: ["n + 1"]}`)),
			`partition "walker"`, "expression.fields[0].name is the boolean false",
			"quote it")
	})
	t.Run("an unquoted output", func(t *testing.T) {
		assertLoadRefused(t, namesRun(walker(
			`{type: expression, fields: [{name: "y"}], outputs: [y]}`)),
			"expression.outputs[0] is the boolean true", "quote it")
	})
	t.Run("an unquoted upstream alias, a mapping key", func(t *testing.T) {
		assertLoadRefused(t, namesRun(walker(
			`{type: expression, upstreams: {on: source}, fields: [{name: x}], outputs: ["x"]}`)),
			"upstreams has a key that YAML reads as the boolean true", "quote the key")
	})
	t.Run("an unquoted field name in a nested run's expressions: block", func(t *testing.T) {
		config := strings.Replace(nestedNamesRun("rate", "gain"),
			`          iteration: {type: expression, fields: [{name: x}], outputs: ["x + 1"]}
`, "", 1)
		config = strings.Replace(config, "        simulation:\n",
			"        expressions:\n        - {partition: inner, fields: [{name: no}], outputs: [\"1\"]}\n"+
				"        simulation:\n", 1)
		assertLoadRefused(t, config, "run.expressions[0].fields[0].name is the boolean false",
			"quote it")
	})
	t.Run("positive control: quoted, the name is the one written", func(t *testing.T) {
		got := walkerValues(t, namesRun(walker(
			`{type: expression, fields: [{name: "n"}], outputs: ["n + 1"]}`)))
		want := []float64{0, 1, 2, 3, 4}
		if !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("walker = %v, want %v", got, want)
		}
	})
}
