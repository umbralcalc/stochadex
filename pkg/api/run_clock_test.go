package api

import (
	"strings"
	"testing"
)

// clockWalk is a one-partition main: run with the given simulation block.
func clockWalk(simulation string) string {
	return "main:\n  partitions:\n" + walkModel + simulation
}

const (
	clockTermination = "    termination_condition: {type: number_of_steps, max_steps: 3}\n"
	clockTimestep    = "    timestep_function: {type: constant, stepsize: 1.0}\n"
)

// clockHostedRun is a main: walk hosting embedded run e, stated by embedded.
func clockHostedRun(embedded string) string {
	return clockWalk("  - {name: e, params: {burn_in_steps: [0]}, init_state_values: [0.0], "+
		"state_history_depth: 1, seed: 0}\n  simulation:\n"+clockTermination+clockTimestep) +
		"embedded:\n- name: e\n" + embedded
}

const embeddedWalk = "  partitions:\n  - {name: inner, iteration: {type: wiener_process}, " +
	"params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 2}\n"

// clockNestedHost is a main: partition running the given run in place.
func clockNestedHost(simulation string) string {
	return clockWalk(`  - name: host
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - {name: inner, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 2}
` + simulation + `    params: {burn_in_steps: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
` + clockTermination + clockTimestep)
}

// assertClockRefused checks a config fails to load as an ErrConfig whose
// message has every want and none of avoid.
func assertClockRefused(t *testing.T, yaml string, want, avoid []string) {
	t.Helper()
	_, err := LoadConfig(writeConfigPath(t, yaml))
	if KindOf(err) != ErrConfig {
		t.Fatalf("expected an ErrConfig at load, got %v", err)
	}
	for _, text := range want {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("error should say %q: %v", text, err)
		}
	}
	for _, text := range avoid {
		if strings.Contains(err.Error(), text) {
			t.Errorf("error should not say %q: %v", text, err)
		}
	}
}

func TestRunClockIsCheckedAtLoad(t *testing.T) {
	t.Run("main.simulation missing a field is refused, naming what is missing", func(t *testing.T) {
		cases := []struct {
			name, simulation string
			want, avoid      []string
		}{
			{"output only", "  simulation:\n    output_function: {type: stdout}\n",
				[]string{"main.simulation needs termination_condition", "and timestep_function"}, nil},
			{"no termination", "  simulation:\n" + clockTimestep,
				[]string{"main.simulation needs termination_condition"}, []string{"and timestep_function"}},
			{"no timestep", "  simulation:\n" + clockTermination,
				[]string{"main.simulation needs timestep_function"}, []string{"needs termination_condition"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertClockRefused(t, clockWalk(c.simulation), c.want, c.avoid)
			})
		}
	})

	t.Run("no main.simulation loads, for Go to supply, but does not run or check", func(t *testing.T) {
		path := writeConfigPath(t, clockWalk(""))
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("a config with no main.simulation should load: %v", err)
		}
		const want = "main.simulation needs termination_condition (e.g. {type: number_of_steps, " +
			"max_steps: 100}) and timestep_function"
		if _, err := RunToStorage(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), want) {
			t.Errorf("expected the run to be an ErrConfig saying %q, got %v", want, err)
		}
		if err, _ := executeIn(t, t.TempDir(), "--config", path, "--check"); KindOf(err) != ErrConfig ||
			!strings.Contains(err.Error(), want) {
			t.Errorf("expected --check to be an ErrConfig saying %q, got %v", want, err)
		}
	})

	t.Run("a whole main.simulation loads and runs", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, clockWalk("  simulation:\n"+clockTermination+clockTimestep)))
		if err != nil {
			t.Fatal(err)
		}
		if steps := len(result.Storage.GetTimes()); steps != 4 {
			t.Errorf("expected the initial row and 3 steps, got %d", steps)
		}
	})

	t.Run("the CLI's --check refuses it as a config error", func(t *testing.T) {
		path := writeConfigPath(t, clockWalk("  simulation:\n    output_function: {type: stdout}\n"))
		err, _ := executeIn(t, t.TempDir(), "--config", path, "--check")
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "main.simulation needs termination_condition") {
			t.Errorf("expected an ErrConfig naming main.simulation's termination_condition, got %v", err)
		}
	})

	t.Run("an embedded run missing a part is refused, naming it", func(t *testing.T) {
		cases := []struct {
			name, yaml string
			want       []string
		}{
			{"no partitions", clockHostedRun("  partitions: []\n  simulation:\n" +
				clockTermination + clockTimestep),
				[]string{`embedded[0] ("e") has no partitions`}},
			{"no host", clockWalk("  simulation:\n"+clockTermination+clockTimestep) +
				"embedded:\n- name: e\n" + embeddedWalk + "  simulation:\n" + clockTermination + clockTimestep,
				[]string{`embedded[0] ("e") has no host`, `partition named "e"`}},
			{"no simulation", clockHostedRun(embeddedWalk),
				[]string{`embedded[0] ("e")'s simulation needs termination_condition`, "and timestep_function"}},
			{"no timestep", clockHostedRun(embeddedWalk + "  simulation:\n" + clockTermination),
				[]string{`embedded[0] ("e")'s simulation needs timestep_function`}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertClockRefused(t, c.yaml, c.want, nil)
			})
		}
	})

	t.Run("a whole embedded run loads and runs", func(t *testing.T) {
		yaml := clockHostedRun(embeddedWalk + "  simulation:\n" + clockTermination + clockTimestep)
		if _, err := RunToStorage(writeConfig(t, yaml)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a nested run missing its clock is refused, naming it", func(t *testing.T) {
		assertClockRefused(t, clockNestedHost(""), []string{"embedded_simulation_run: ",
			"run.simulation needs termination_condition", "and timestep_function"}, nil)
		assertClockRefused(t, clockNestedHost("        simulation:\n          timestep_function: {type: constant, stepsize: 1.0}\n"),
			[]string{"run.simulation needs termination_condition"}, []string{"and timestep_function"})
		if _, err := RunToStorage(writeConfig(t, clockNestedHost("        simulation:\n"+
			"          termination_condition: {type: number_of_steps, max_steps: 2}\n"+
			"          timestep_function: {type: constant, stepsize: 1.0}\n"))); err != nil {
			t.Fatalf("a whole nested run should run: %v", err)
		}
	})

	t.Run("a replicated run missing its clock is refused, naming it", func(t *testing.T) {
		spec := strings.Replace(replicatedSpec,
			"          termination_condition: {type: number_of_steps, max_steps: 4}\n", "", 1)
		assertClockRefused(t, replicatedYAML(spec), []string{"replicated_simulation_run: ",
			"run.simulation needs termination_condition"}, []string{"and timestep_function"})
	})
}

func TestMainClockBesideMacros(t *testing.T) {
	t.Run("a partial main.simulation is refused as the run's clock", func(t *testing.T) {
		const where = "main.simulation, which beside macros: is the run's clock " +
			"(an output goes under outputs:), needs "
		cases := []struct{ name, simulation, missing string }{
			{"output only", "    output_function: {type: stdout}\n", "termination_condition"},
			{"init time only", "    init_time_value: 5.0\n", "termination_condition"},
			{"no timestep", clockTermination, "timestep_function"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertClockRefused(t, macroConfigYAML+"main:\n  simulation:\n"+c.simulation,
					[]string{where + c.missing}, nil)
			})
		}
	})

	t.Run("no main.simulation leaves the clock to the inputs", func(t *testing.T) {
		if _, err := RunToStorage(writeConfig(t, macroConfigYAML)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a whole main.simulation beside macros is the run's clock", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, macroConfigYAML+"main:\n  simulation:\n"+
			"    termination_condition: {type: number_of_steps, max_steps: 7}\n"+clockTimestep))
		if err != nil {
			t.Fatal(err)
		}
		if steps := len(result.Storage.GetTimes()); steps != 8 {
			t.Errorf("expected the initial row and main.simulation's 7 steps, got %d", steps)
		}
	})
}
