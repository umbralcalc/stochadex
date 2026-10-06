package api

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// nestedSinksYAML is a run with three sinks under dir: two top-level output
// views (every step to all.log; partition w only to w.log) and a json_log
// inside a nested (embedded) run (nested.log).
func nestedSinksYAML(dir string) string {
	return fmt.Sprintf(`main:
  partitions:
  - name: nested
    params: {burn_in_steps: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  - name: w
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 1
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 6}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: all, function: {type: json_log, path: %q}}
- {name: w, condition: {type: only_given_partitions, partitions: [w]}, function: {type: json_log, path: %q}}
embedded:
- name: nested
  partitions:
  - name: inner
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [5.0]
    state_history_depth: 1
    seed: 11
  simulation:
    output_condition: {type: every_step}
    output_function: {type: json_log, path: %q}
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, filepath.Join(dir, "all.log"), filepath.Join(dir, "w.log"), filepath.Join(dir, "nested.log"))
}

var sinkFiles = []string{"all.log", "w.log", "nested.log"}

// storageEntries indexes a storage's recorded rows by (time, partition), the
// same keying keyedEntries uses for a json_log.
func storageEntries(storage *simulator.StateTimeStorage) map[string][]float64 {
	byKey := map[string][]float64{}
	times := storage.GetTimes()
	for _, name := range storage.GetNames() {
		for step, row := range storage.GetValues(name) {
			byKey[fmt.Sprintf("%v/%s", times[step], name)] = row
		}
	}
	return byKey
}

func TestRunWith(t *testing.T) {
	t.Run("each captured view records what a json_log view with its condition writes", func(t *testing.T) {
		conditions := map[string]simulator.OutputCondition{
			"all":    &simulator.EveryStepOutputCondition{},
			"w":      &simulator.OnlyGivenPartitionsOutputCondition{Partitions: map[string]bool{"w": true}},
			"sparse": &simulator.EveryNStepsOutputCondition{N: 2},
		}
		options := []RunOption{}
		for name, condition := range conditions {
			options = append(options, CaptureView(name, condition))
		}
		result, err := RunWith(writeConfig(t, nestedSinksYAML(t.TempDir())), options...)
		if err != nil {
			t.Fatal(err)
		}
		// Reference: the same model run normally, its outputs: views writing files.
		// A sparse view is added to the reference so every capture has a file.
		referenceDir := t.TempDir()
		yaml := replaceOnce(t, nestedSinksYAML(referenceDir),
			fmt.Sprintf("- {name: w, condition: {type: only_given_partitions, partitions: [w]}, function: {type: json_log, path: %q}}\n",
				filepath.Join(referenceDir, "w.log")),
			fmt.Sprintf("- {name: w, condition: {type: only_given_partitions, partitions: [w]}, function: {type: json_log, path: %q}}\n"+
				"- {name: sparse, condition: {type: every_n_steps, n: 2}, function: {type: json_log, path: %q}}\n",
				filepath.Join(referenceDir, "w.log"), filepath.Join(referenceDir, "sparse.log")))
		Run(writeConfig(t, yaml), &SocketConfig{})
		for name := range conditions {
			assertSameEntries(t, "captured view "+name,
				storageEntries(result.Views[name]),
				keyedEntries(t, filepath.Join(referenceDir, name+".log")))
		}
	})

	t.Run("by default nothing is written, nested sinks included", func(t *testing.T) {
		dir := t.TempDir()
		for _, file := range sinkFiles {
			writeSentinel(t, filepath.Join(dir, file))
		}
		config := writeConfig(t, nestedSinksYAML(dir))
		if _, err := RunWith(config, CaptureView("all", nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := RunToStorage(config); err != nil {
			t.Fatal(err)
		}
		for _, file := range sinkFiles {
			assertSentinelIntact(t, filepath.Join(dir, file), "RunWith / RunToStorage")
		}
		// Positive control: a normal run does write every sink, the nested one
		// included (6 outer steps x 5 inner rows).
		Run(config, &SocketConfig{})
		if n := len(readLogEntries(t, filepath.Join(dir, "nested.log"))); n != 30 {
			t.Errorf("a normal run should write 30 nested entries, got %d", n)
		}
	})

	t.Run("WithConfigOutputs writes exactly what Run writes, and still captures", func(t *testing.T) {
		teeDir, runDir := t.TempDir(), t.TempDir()
		result, err := RunWith(writeConfig(t, nestedSinksYAML(teeDir)),
			CaptureView("all", nil), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		Run(writeConfig(t, nestedSinksYAML(runDir)), &SocketConfig{})
		for _, file := range []string{"all.log", "w.log"} {
			assertSameEntries(t, file,
				keyedEntries(t, filepath.Join(teeDir, file)),
				keyedEntries(t, filepath.Join(runDir, file)))
		}
		// Nested records repeat (time, partition) across inner runs — they carry
		// no outer-step scope yet (PLAN.md §2.5, Q7) — so compare them in order:
		// one partition, written sequentially, so the order is deterministic.
		tee, run := readLogEntries(t, filepath.Join(teeDir, "nested.log")),
			readLogEntries(t, filepath.Join(runDir, "nested.log"))
		if len(tee) != 30 || len(tee) != len(run) {
			t.Fatalf("nested.log: %d entries tee'd, %d from Run, want 30 each", len(tee), len(run))
		}
		for i := range run {
			if tee[i].CumulativeTimesteps != run[i].CumulativeTimesteps ||
				!floats.Equal(tee[i].State, run[i].State) {
				t.Fatalf("nested.log entry %d = %+v, want %+v", i, tee[i], run[i])
			}
		}
		assertSameEntries(t, "captured all",
			storageEntries(result.Views["all"]), keyedEntries(t, filepath.Join(runDir, "all.log")))
	})

	t.Run("WithConfigOutputs also writes the shorthand output pair", func(t *testing.T) {
		// Most configs declare output as output_condition / output_function rather
		// than outputs:; teeing must write that sink exactly as Run does.
		condition := "{type: only_given_partitions, partitions: [second]}"
		teePath := filepath.Join(t.TempDir(), "second.log")
		runPath := filepath.Join(t.TempDir(), "second.log")
		result, err := RunWith(writeConfig(t, fmt.Sprintf(batchConfigYAML, condition, teePath)),
			CaptureView("all", nil), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		Run(writeConfig(t, fmt.Sprintf(batchConfigYAML, condition, runPath)), &SocketConfig{})
		assertSameEntries(t, "tee'd shorthand sink", keyedEntries(t, teePath), keyedEntries(t, runPath))
		if n := len(storageEntries(result.Views["all"])); n != 62 {
			t.Errorf("the captured view should still see every row (62), got %d", n)
		}
	})

	t.Run("a config without any output still runs with WithConfigOutputs", func(t *testing.T) {
		yaml := replaceOnce(t, fmt.Sprintf(batchConfigYAML, "{type: every_step}", "unused"),
			"    output_condition: {type: every_step}\n    output_function: {type: json_log, path: \"unused\"}\n", "")
		result, err := RunWith(writeConfig(t, yaml), CaptureView("all", nil), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		if n := len(storageEntries(result.Views["all"])); n != 62 {
			t.Errorf("captured %d rows, want 62", n)
		}
	})

	t.Run("the config is unchanged and can be run again with the same result", func(t *testing.T) {
		config := writeConfig(t, nestedSinksYAML(t.TempDir()))
		outputBefore := config.Main.Simulation.OutputFunction
		first, err := RunWith(config, CaptureView("all", nil))
		if err != nil {
			t.Fatal(err)
		}
		if config.Main.Simulation.OutputFunction != outputBefore {
			t.Fatal("RunWith replaced the config's own output function")
		}
		second, err := RunWith(config, CaptureView("all", nil))
		if err != nil {
			t.Fatal(err)
		}
		assertSameEntries(t, "second run", storageEntries(second.Views["all"]),
			storageEntries(first.Views["all"]))
	})

	t.Run("a macros config: views are fed from its result, config views follow the same rule", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "var.log")
		yaml := macroConfigYAML + fmt.Sprintf(
			"outputs:\n- {name: var, condition: {type: only_given_partitions, partitions: [rolling_var]}, function: {type: json_log, path: %q}}\n", path)
		writeSentinel(t, path)
		result, err := RunWith(writeConfig(t, yaml), CaptureView("var",
			&simulator.OnlyGivenPartitionsOutputCondition{Partitions: map[string]bool{"rolling_var": true}}))
		if err != nil {
			t.Fatal(err)
		}
		assertSentinelIntact(t, path, "RunWith on a macros config")
		// The captured view holds exactly the result's rolling_var rows.
		captured := storageEntries(result.Views["var"])
		want := map[string][]float64{}
		for key, values := range storageEntries(result.Storage) {
			if strings.HasSuffix(key, "/rolling_var") {
				want[key] = values
			}
		}
		if len(want) != 501 {
			t.Fatalf("expected 501 rolling_var rows in the result, got %d", len(want))
		}
		assertSameEntries(t, "captured var view", captured, want)

		teePath := filepath.Join(t.TempDir(), "var.log")
		teeYAML := replaceOnce(t, yaml, path, teePath)
		if _, err := RunWith(writeConfig(t, teeYAML), WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}
		runPath := filepath.Join(t.TempDir(), "var.log")
		var runErr error
		captureStdout(t, func() {
			runErr = runChecked(writeConfig(t, replaceOnce(t, yaml, path, runPath)), &SocketConfig{})
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		assertSameEntries(t, "tee'd macro view", keyedEntries(t, teePath), keyedEntries(t, runPath))
	})

	t.Run("misuse is a usage error", func(t *testing.T) {
		_, err := RunWith(writeConfig(t, nestedSinksYAML(t.TempDir())),
			CaptureView("x", nil), CaptureView("x", nil))
		if KindOf(err) != ErrUsage {
			t.Errorf("duplicate capture names: expected ErrUsage, got %v", err)
		}
		ensemble := readFile(t, "../../cfg/example_ensemble_config.yaml")
		for name, option := range map[string]RunOption{
			"a captured view": CaptureView("x", nil), "WithConfigOutputs": WithConfigOutputs(),
		} {
			if _, err := RunWith(writeConfig(t, ensemble), option); KindOf(err) != ErrUsage {
				t.Errorf("%s on an ensemble: expected ErrUsage, got %v", name, err)
			}
		}
		members, err := RunWith(writeConfig(t, ensemble))
		if err != nil || len(members.Members) != 4 {
			t.Errorf("an ensemble without options should still return its 4 members, got %v, %v",
				members, err)
		}
	})
}
