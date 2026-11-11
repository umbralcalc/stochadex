package api

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// nestedChainYAML hosts a 10-step run whose partition hosts a 5-step run whose
// partition hosts a 2-step run, beside a run that stops at a time rather than a
// step count, which hosts a 2-step run.
const nestedChainYAML = `main:
  partitions:
  - name: outer
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - name: inner
          iteration:
            type: embedded_simulation_run
            run:
              partitions:
              - name: leaf
                iteration:
                  type: embedded_simulation_run
                  run:
                    partitions:
                    - {name: bottom, iteration: {type: wiener_process}, params: {variances: [1.0, 1.0]}, init_state_values: [0.0, 1.0], state_history_depth: 1, seed: 1}
                    simulation:
                      termination_condition: {type: number_of_steps, max_steps: 2}
                      timestep_function: {type: constant, stepsize: 1.0}
                      init_time_value: 0.0
                params: {burn_in_steps: [0]}
                init_state_values: [0.0, 1.0]
                state_history_depth: 1
                seed: 0
              simulation:
                termination_condition: {type: number_of_steps, max_steps: 5}
                timestep_function: {type: constant, stepsize: 1.0}
                init_time_value: 0.0
          params: {burn_in_steps: [0]}
          init_state_values: [0.0, 1.0]
          state_history_depth: 1
          seed: 0
        simulation:
          termination_condition: {type: number_of_steps, max_steps: 10}
          timestep_function: {type: constant, stepsize: 1.0}
          init_time_value: 0.0
    params: {burn_in_steps: [0]}
    init_state_values: [0.0, 1.0]
    state_history_depth: 1
    seed: 0
  - name: timed
    iteration:
      type: embedded_simulation_run
      run:
        partitions:
        - name: counted
          iteration:
            type: embedded_simulation_run
            run:
              partitions:
              - {name: tick, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 2}
              simulation:
                termination_condition: {type: number_of_steps, max_steps: 2}
                timestep_function: {type: constant, stepsize: 1.0}
                init_time_value: 0.0
          params: {burn_in_steps: [0]}
          init_state_values: [0.0]
          state_history_depth: 1
          seed: 0
        simulation:
          termination_condition: {type: time_elapsed, max_time_elapsed: 3.0}
          timestep_function: {type: constant, stepsize: 0.5}
          init_time_value: 0.0
    params: {burn_in_steps: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

func TestManifestNestingCost(t *testing.T) {
	asJSON := func(v any) string {
		out, _ := json.MarshalIndent(v, "", "  ")
		return string(out)
	}

	t.Run("a chain of runs lists each with its steps, multiplied down the chain", func(t *testing.T) {
		config := mustLoad(t, writeConfigPath(t, nestedChainYAML))
		want := []ManifestEmbedded{
			{Partition: "outer", Depth: 1, Termination: "number_of_steps", Steps: intPointer(10),
				StepsPerRunStep: intPointer(10), Columns: []ManifestColumns{{Partition: "inner", Width: 2}}},
			{Partition: "outer/inner", Depth: 2, Termination: "number_of_steps", Steps: intPointer(5),
				StepsPerRunStep: intPointer(50), Columns: []ManifestColumns{{Partition: "leaf", Width: 2}}},
			{Partition: "outer/inner/leaf", Depth: 3, Termination: "number_of_steps", Steps: intPointer(2),
				StepsPerRunStep: intPointer(100), Columns: []ManifestColumns{{Partition: "bottom", Width: 2}}},
			// A run that stops at a time has no fixed count, and nor does what it hosts.
			{Partition: "timed", Depth: 1, Termination: "time_elapsed",
				Columns: []ManifestColumns{{Partition: "counted", Width: 1}}},
			{Partition: "timed/counted", Depth: 2, Termination: "number_of_steps", Steps: intPointer(2),
				Columns: []ManifestColumns{{Partition: "tick", Width: 1}}},
		}
		if got := Manifest(config).Embedded; asJSON(got) != asJSON(want) {
			t.Errorf("embedded\n%s\nwant\n%s", asJSON(got), asJSON(want))
		}
		if _, err := RunToStorage(config); err != nil {
			t.Fatalf("the chain does not run: %v", err)
		}
	})

	t.Run("a replicated run counts each replica, and its columns are its output's", func(t *testing.T) {
		config := mustLoad(t, writeConfigPath(t, replicatedYAML(replicatedSpec)))
		want := []ManifestEmbedded{{Partition: "replicas", Depth: 1, Termination: "number_of_steps",
			Steps: intPointer(4), Replicas: 3, StepsPerRunStep: intPointer(12),
			Columns: []ManifestColumns{{Partition: "echo", Width: 2}}}}
		if got := Manifest(config).Embedded; asJSON(got) != asJSON(want) {
			t.Errorf("embedded\n%s\nwant\n%s", asJSON(got), asJSON(want))
		}
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		// replica r's columns are replica 0's, offset r times their width.
		if rows := result.Storage.GetValues("replicas"); len(rows[1]) != 3*2 {
			t.Errorf("the host row is %d wide, want 3 replicas of 2", len(rows[1]))
		}
	})

	t.Run("a run a replica's model hosts is listed under it, counted per replica", func(t *testing.T) {
		model := strings.Replace(replicatedSpec, "        - name: echo\n", `        - name: sub
          iteration:
            type: embedded_simulation_run
            run:
              partitions:
              - {name: s, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 3}
              simulation:
                termination_condition: {type: number_of_steps, max_steps: 6}
                timestep_function: {type: constant, stepsize: 1.0}
                init_time_value: 0.0
          params: {burn_in_steps: [0]}
          init_state_values: [0.0]
          state_history_depth: 1
          seed: 0
        - name: echo
`, 1)
		config := mustLoad(t, writeConfigPath(t, replicatedYAML(model)))
		embedded := Manifest(config).Embedded
		// 3 replicas of 4 steps, each step running the sub-run's 6.
		want := ManifestEmbedded{Partition: "replicas/sub", Depth: 2, Termination: "number_of_steps",
			Steps: intPointer(6), StepsPerRunStep: intPointer(3 * 4 * 6),
			Columns: []ManifestColumns{{Partition: "s", Width: 1}}}
		if len(embedded) != 2 || asJSON(embedded[1]) != asJSON(want) {
			t.Errorf("embedded\n%s\nwant second\n%s", asJSON(embedded), asJSON(want))
		}
		if _, err := RunToStorage(config); err != nil {
			t.Fatalf("the replicated model does not run: %v", err)
		}
	})

	t.Run("steps match the inner steps a run actually takes, as its debug log records them", func(t *testing.T) {
		debug := filepath.Join(t.TempDir(), "inner.log")
		option, err := debugEmbeddedOption("host=" + debug)
		if err != nil {
			t.Fatal(err)
		}
		config, err := LoadConfig(writeConfigPath(t, blackBoxYAML), option)
		if err != nil {
			t.Fatal(err)
		}
		manifest := Manifest(config)
		captureStdout(t, func() { _, err = RunWith(config, WithConfigOutputs()) })
		if err != nil {
			t.Fatal(err)
		}
		// Each inner run logs its initial row and a row per step, for each inner partition.
		const outerSteps = 5
		entry := manifest.Embedded[0]
		perRun := strings.Count(readAll(t, debug), "\n") / (outerSteps * len(entry.Columns))
		if entry.Steps == nil || *entry.Steps != perRun-1 || *entry.StepsPerRunStep != perRun-1 {
			t.Errorf("manifest steps %v / %v, but each inner run took %d", entry.Steps, entry.StepsPerRunStep, perRun-1)
		}
	})
}
