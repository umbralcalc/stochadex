package api

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestEveryRunReadsItsInputs: one loaded config, run twice with its input
// file rewritten in between, reads the file as it is at each run, in every
// place a run reads an input, and the caller's config is left as loaded.
func TestEveryRunReadsItsInputs(t *testing.T) {
	csv := filepath.Join(t.TempDir(), "x.csv")
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(csv, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("0,1\n1,2\n2,3\n")
	config, err := LoadConfig(writeConfigPath(t, fmt.Sprintf(`inputs:
  d: {source: {csv: {path: %q, time_column: 0, state_columns: {x: [1]}}}}
main:
  partitions:
  - {name: x, iteration: {type: from_input, input: d}, state_history_depth: 1, seed: 0}
  - name: fed
    iteration: {type: param_values}
    params: {param_values: [0.0]}
    params_from_input: {param_values: {input: d, partition: x}}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  - name: nested
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - {name: inner, iteration: {type: from_input, input: d, partition: x}, init_state_values: [1.0], state_history_depth: 1, seed: 0}
        simulation:
          termination_condition: {type: input_exhausted, input: d}
          timestep_function: {type: from_input, input: d}
          init_time_value: 0.0
    params: {burn_in_steps: [0]}
    init_state_values: [1.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: input_exhausted, input: d}
    timestep_function: {type: from_input, input: d}
    init_time_value: 0.0
`, csv)))
	if err != nil {
		t.Fatal(err)
	}
	run := func() (rows, fed, nested, times string) {
		t.Helper()
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		storage := result.Storage
		return fmt.Sprint(storage.GetValues("x")), fmt.Sprint(storage.GetValues("fed")),
			fmt.Sprint(storage.GetValues("nested")), fmt.Sprint(storage.GetTimes())
	}
	check := func(label, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s: %s, want %s", label, got, want)
		}
	}

	rows, fed, nested, times := run()
	check("first run's replay", rows, "[[1] [2] [3]]")
	check("first run's params_from_input", fed, "[[0] [2] [3]]")
	check("first run's nested replay (the input's last row, each step)", nested, "[[1] [3] [3]]")
	check("first run's clock", times, "[0 1 2]")

	write("0,10\n2,20\n5,30\n")
	rows, fed, nested, times = run()
	check("second run's replay", rows, "[[10] [20] [30]]")
	check("second run's params_from_input", fed, "[[0] [20] [30]]")
	check("second run's nested replay", nested, "[[1] [30] [30]]")
	check("second run's clock", times, "[0 2 5]")

	if _, unbound := config.Main.Partitions[0].Iteration.(*unboundInputIteration); !unbound {
		t.Errorf("the caller's config was bound: its replay is now %T", config.Main.Partitions[0].Iteration)
	}
}
