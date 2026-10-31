package api

import (
	"fmt"
	"strings"
	"testing"
)

// walkModel is a noisy walk, written as main: partitions or as an input's
// sub-simulation.
const walkModel = `  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 3}
`

// walkMain runs walkModel as main:, for 60 steps of 1, with extra partitions
// after it and the given macros.
func walkMain(extra, macros string) string {
	return "main:\n  partitions:\n" + walkModel + extra + `  simulation:
    termination_condition: {type: number_of_steps, max_steps: 60}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
macros:
` + macros
}

// walkRecorded is the same walk recorded as an input, under the given macros.
func walkRecorded(macros string) string {
	return `inputs:
  data:
    simulation:
      steps: 60
      timestep: 1.0
      partitions:
` + strings.ReplaceAll(walkModel, "  - ", "      - ") + "macros:\n" + macros
}

// composedRun loads and runs a config, as RunToStorage runs it by default.
func composedRun(t *testing.T, yaml string) *RunResult {
	t.Helper()
	result, err := RunToStorage(writeConfig(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMainBesideMacros(t *testing.T) {
	const mean = "- {type: vector_mean, name: m, data: {partition_name: walk}, window: 5}\n"

	t.Run("a macro reads a main: partition live as it reads one recorded", func(t *testing.T) {
		live := composedRun(t, walkMain("", mean)).Storage
		recorded, err := RunMacros(LoadApiRunConfigFromYaml(writeConfigPath(t, walkRecorded(mean))))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "live vs recorded", live, recorded)
	})

	t.Run("its window deepens the main: partition's history", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, walkMain("", mean)))
		if err != nil {
			t.Fatal(err)
		}
		written, err := ExpandToYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(written), "{name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 5, seed: 3}") {
			t.Errorf("the walk's history is not the window's 5 deep:\n%s", written)
		}
		deeper, err := LoadConfig(writeConfigPath(t, strings.Replace(walkMain("", mean),
			"state_history_depth: 1, seed: 3", "state_history_depth: 9, seed: 3", 1)))
		if err != nil {
			t.Fatal(err)
		}
		if written, err = ExpandToYAML(deeper); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(written), "state_history_depth: 9, seed: 3}") {
			t.Errorf("a history already deeper than the window was changed:\n%s", written)
		}
	})

	t.Run("a main: partition reads a macro's output within the step", func(t *testing.T) {
		storage := composedRun(t, walkMain(`  - name: doubled
    iteration: {type: expression, fields: [{name: d}], outputs: ["2 * v[0]"]}
    params: {v: [0.0]}
    params_from_upstream: {v: {upstream: m}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
`, mean)).Storage
		doubled, means := storage.GetValues("doubled"), storage.GetValues("m")
		for i := range means {
			if doubled[i][0] != 2*means[i][0] {
				t.Fatalf("step %d: doubled %v, want twice the mean %v", i, doubled[i][0], means[i][0])
			}
		}
	})

	t.Run("main: expressions run beside macros", func(t *testing.T) {
		storage := composedRun(t, strings.Replace(walkMain(`  - {name: twice, params: {v: [0.0]}, params_from_upstream: {v: {upstream: walk}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
`, mean), "  simulation:\n", "  expressions:\n  - {partition: twice, fields: [{name: x}], outputs: [\"2 * v[0]\"]}\n  simulation:\n", 1)).Storage
		twice, walk := storage.GetValues("twice"), storage.GetValues("walk")
		for i := 1; i < len(walk); i++ {
			if twice[i][0] != 2*walk[i][0] {
				t.Fatalf("step %d: twice %v, want twice the walk %v", i, twice[i][0], walk[i][0])
			}
		}
	})

	t.Run("main: partitions read the inputs' replays, on the inputs' clock", func(t *testing.T) {
		storage := composedRun(t, strings.Replace(walkRecorded(
			"- {type: vector_mean, name: m, data: {partition_name: tripled}, window: 2}\n"),
			"macros:\n", `main:
  partitions:
  - name: tripled
    iteration: {type: expression, fields: [{name: x}], outputs: ["3 * w[0]"]}
    params: {w: [0.0]}
    params_from_upstream: {w: {upstream: walk}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
macros:
`, 1)).Storage
		tripled, walk := storage.GetValues("tripled"), storage.GetValues("walk")
		if len(walk) != 61 {
			t.Fatalf("got %d rows, want the input's 61", len(walk))
		}
		for i := 1; i < len(walk); i++ {
			if tripled[i][0] != 3*walk[i][0] {
				t.Fatalf("step %d: tripled %v, want three times the walk %v", i, tripled[i][0], walk[i][0])
			}
		}
	})

	t.Run("a macro with its own clock steps main: partitions", func(t *testing.T) {
		es := readFile(t, "../../cfg/example_evolution_strategy_config.yaml")
		storage := composedRun(t, strings.Replace(es, "steps: 1000", "steps: 30", 1)+`main:
  partitions:
  - name: watch
    iteration: {type: param_values}
    params: {param_values: [0.0, 0.0]}
    params_from_upstream: {param_values: {upstream: es_mean}}
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: 0
`).Storage
		watch, means := storage.GetValues("watch"), storage.GetValues("es_mean")
		if len(means) != 31 {
			t.Fatalf("got %d rows, want the macro's 31", len(means))
		}
		for i := 1; i < len(means); i++ {
			if fmt.Sprint(watch[i]) != fmt.Sprint(means[i]) {
				t.Fatalf("step %d: watch %v, want es_mean %v", i, watch[i], means[i])
			}
		}
	})

	t.Run("an embedded: run a main: partition hosts runs beside macros as without them", func(t *testing.T) {
		hosted := `  - {name: inner_run, params: {burn_in_steps: [0]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
`
		embedded := `embedded:
- name: inner_run
  partitions:
  - {name: inner, iteration: {type: wiener_process}, params: {variances: [0.5]}, init_state_values: [0.0], state_history_depth: 1, seed: 9}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
		beside := composedRun(t, walkMain(hosted, mean)+embedded).Storage
		alone := composedRun(t, strings.TrimSuffix(walkMain(hosted, ""), "macros:\n")+embedded).Storage
		if got, want := fmt.Sprint(beside.GetValues("inner_run")), fmt.Sprint(alone.GetValues("inner_run")); got != want {
			t.Errorf("beside macros the embedded run gave %s, alone %s", got, want)
		}
	})

	t.Run("--check passes, and the CLI runs it", func(t *testing.T) {
		path := writeConfigPath(t, walkMain("", mean))
		if err, _ := executeIn(t, t.TempDir(), "--config", path, "--check"); err != nil {
			t.Errorf("--check: %v", err)
		}
		if err, _ := executeIn(t, t.TempDir(), "--config", path); err != nil {
			t.Errorf("run: %v", err)
		}
	})

	for _, c := range []struct{ name, yaml, want string }{
		{"a main: partition named as an input's", strings.Replace(walkRecorded(mean), "macros:\n",
			"main:\n  partitions:\n"+walkModel+"macros:\n", 1),
			`main: defines partition "walk", which input "data" has too`},
		{"a macro naming its partition as a main: one", walkMain(`  - {name: m, iteration: {type: constant_values}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
`, mean), `macros[0] (vector_mean) names its partition "m", which main:, the inputs or an earlier macro already have`},
		{"main.simulation beside a macro with its own clock",
			"main:\n  simulation:\n    termination_condition: {type: number_of_steps, max_steps: 3}\n" +
				"    timestep_function: {type: constant, stepsize: 1.0}\n" +
				readFile(t, "../../cfg/example_mcts_config.yaml"),
			"runs its own clock, and main.simulation states one too"},
		{"main: partitions beside calibrate, then plan", strings.Replace(calibration, "macros:\n",
			"main:\n  partitions:\n"+strings.Replace(walkModel, "walk", "extra", 1)+"macros:\n", 1) +
			strings.Replace(newsvendorPlan, "PARAMETERS", "samples_from: {partition_name: post_sampler, burn_in: 100}", 1),
			"main: partitions beside macros that calibrate before a macro with its own clock"},
	} {
		t.Run(c.name+" does not run, saying why", func(t *testing.T) {
			_, err := RunToStorage(writeConfig(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("macro by macro, main: beside macros is still refused", func(t *testing.T) {
		_, err := RunMacros(LoadApiRunConfigFromYaml(writeConfigPath(t, walkMain("", mean))))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "sets both main.partitions and macros:") {
			t.Errorf("got %v", err)
		}
	})
}
