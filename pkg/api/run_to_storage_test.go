package api

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// batchConfigYAML is a seeded two-partition batch run whose output_function is a
// json_log sink at %s, so the sink path and the storage path can be compared.
const batchConfigYAML = `main:
  partitions:
  - name: first
    iteration: {type: wiener_process}
    params: {variances: [1.0, 2.0]}
    init_state_values: [0.0, 1.0]
    state_history_depth: 1
    seed: 7167
  - name: second
    iteration: {type: wiener_process}
    params: {variances: [0.5]}
    init_state_values: [3.0]
    state_history_depth: 1
    seed: 2939
  simulation:
    output_condition: %s
    output_function: {type: json_log, path: "%s"}
    termination_condition: {type: number_of_steps, max_steps: 30}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

// assertStoragesEqual fails unless both storages hold the same partitions, times
// and rows, value for value.
func assertStoragesEqual(t *testing.T, label string, got, want *simulator.StateTimeStorage) {
	t.Helper()
	gotNames, wantNames := got.GetNames(), want.GetNames()
	sort.Strings(gotNames)
	sort.Strings(wantNames)
	if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("%s: partitions %v, want %v", label, gotNames, wantNames)
	}
	if !floats.Equal(got.GetTimes(), want.GetTimes()) {
		t.Fatalf("%s: times differ: %v vs %v", label, got.GetTimes(), want.GetTimes())
	}
	for _, name := range wantNames {
		gotRows, wantRows := got.GetValues(name), want.GetValues(name)
		if len(gotRows) != len(wantRows) {
			t.Fatalf("%s: %s has %d rows, want %d", label, name, len(gotRows), len(wantRows))
		}
		for i := range wantRows {
			if !floats.Equal(gotRows[i], wantRows[i]) {
				t.Fatalf("%s: %s row %d = %v, want %v", label, name, i, gotRows[i], wantRows[i])
			}
		}
	}
}

// captureStdout runs f and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	done := make(chan string)
	go func() {
		var buffer bytes.Buffer
		io.Copy(&buffer, reader)
		done <- buffer.String()
	}()
	f()
	writer.Close()
	os.Stdout = original
	return <-done
}

func TestRunToStorage(t *testing.T) {
	t.Run("a batch run returns exactly what its configured sink records", func(t *testing.T) {
		// Reference: the config's own json_log sink, written by Run and read back from
		// disk — an independent output path from the storage RunToStorage fills.
		logPath := filepath.Join(t.TempDir(), "run.log")
		config := writeConfig(t, fmt.Sprintf(batchConfigYAML, "{type: every_step}", logPath))

		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		if result.Members != nil || result.Storage == nil {
			t.Fatalf("a batch run should set Storage only, got %+v", result)
		}
		// Loading the config creates the sink's file (json_log opens it at load
		// time), so the promise is that RunToStorage writes nothing into it.
		if info, err := os.Stat(logPath); err == nil && info.Size() != 0 {
			t.Fatalf("RunToStorage must not invoke the configured output_function, "+
				"but %s has %d bytes", logPath, info.Size())
		}

		// The same config object, run afterwards through Run, must still emit through
		// its own sink — RunToStorage left it unmodified.
		Run(config, &SocketConfig{})
		fromSink, err := analysis.NewStateTimeStorageFromJsonLogEntries(logPath)
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "storage vs json_log sink", result.Storage, fromSink)
		if got := len(result.Storage.GetTimes()); got != 31 {
			t.Errorf("expected the initial state plus 30 steps (31 rows), got %d", got)
		}
	})

	t.Run("the config's output_condition decides what is recorded", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "unused.log")
		only := writeConfig(t, fmt.Sprintf(batchConfigYAML,
			"{type: only_given_partitions, partitions: [second]}", logPath))
		result, err := RunToStorage(only)
		if err != nil {
			t.Fatal(err)
		}
		// Every partition is registered in storage, but only selected ones get rows.
		if rows := result.Storage.GetValues("first"); len(rows) != 0 {
			t.Errorf("only_given_partitions [second] recorded %d rows for first", len(rows))
		}
		if rows := result.Storage.GetValues("second"); len(rows) != 31 {
			t.Errorf("expected 31 rows for second, got %d", len(rows))
		}

		everyAll := writeConfig(t, fmt.Sprintf(batchConfigYAML, "{type: every_step}", logPath))
		all, err := RunToStorage(everyAll)
		if err != nil {
			t.Fatal(err)
		}
		// The selected partition's rows match the unfiltered run of the same seeds.
		want := all.Storage.GetValues("second")
		got := result.Storage.GetValues("second")
		for i := range want {
			if !floats.Equal(got[i], want[i]) {
				t.Fatalf("filtered row %d = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("a macros config returns the same storage as RunMacros", func(t *testing.T) {
		result, err := RunToStorage(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		reference, err := RunMacros(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if result.Members != nil {
			t.Fatal("a macros run should set Storage only")
		}
		assertStoragesEqual(t, "RunToStorage vs RunMacros", result.Storage, reference)
	})

	t.Run("an ensemble config returns members aligned to run.seeds", func(t *testing.T) {
		result, err := RunToStorage(LoadApiRunConfigFromYaml("../../cfg/example_ensemble_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		reference, err := RunEnsembleToStorage(LoadApiRunConfigFromYaml("../../cfg/example_ensemble_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if result.Storage != nil || len(result.Members) != 4 {
			t.Fatalf("an ensemble run should set Members (4) only, got %+v", result)
		}
		for i, seed := range []uint64{11, 22, 33, 44} {
			if result.Members[i].Seed != seed {
				t.Errorf("member %d has seed %d, want %d", i, result.Members[i].Seed, seed)
			}
			assertStoragesEqual(t, fmt.Sprintf("member %d", i),
				result.Members[i].Storage, reference[i].Storage)
		}
		first := result.Members[0].Storage.GetValues("growth")
		second := result.Members[1].Storage.GetValues("growth")
		if floats.Equal(first[len(first)-1], second[len(second)-1]) {
			t.Error("members with different seeds should differ")
		}
	})

	// Loading a config creates its json_log file, so even error cases need a
	// scratch path rather than one relative to the package directory.
	scratchLog := filepath.Join(t.TempDir(), "error-case.log")
	errorCases := []struct {
		name, yaml, wantInError string
	}{
		{"unknown run mode",
			strings.Replace(fmt.Sprintf(batchConfigYAML, "{type: every_step}", scratchLog),
				"main:", "run: {mode: sweep}\nmain:", 1),
			"unknown run mode"},
		{"data: without macros:",
			fmt.Sprintf(batchConfigYAML, "{type: every_step}", scratchLog) +
				"data:\n  steps: 5\n  partitions: []\n",
			"data:"},
		{"ensemble with no seeds",
			strings.Replace(fmt.Sprintf(batchConfigYAML, "{type: every_step}", scratchLog),
				"main:", "run: {mode: ensemble}\nmain:", 1),
			"run.seeds"},
		{"macros alongside an ensemble run:",
			macroConfigYAML + "run: {mode: ensemble, seeds: [1]}\n",
			"run:"},
		{"a within-step dependency cycle",
			`main:
  partitions:
  - {name: a, iteration: {type: copy_values}, params: {partitions: [1], partition_state_values: [0]}, params_from_upstream: {partition_state_values: {upstream: b}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: b, iteration: {type: copy_values}, params: {partitions: [0], partition_state_values: [0]}, params_from_upstream: {partition_state_values: {upstream: a}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`,
			"deadlock"},
	}
	for _, c := range errorCases {
		t.Run(c.name+" is returned as an error, naming the problem", func(t *testing.T) {
			_, err := RunToStorage(writeConfig(t, c.yaml))
			if err == nil {
				t.Fatalf("expected an error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.wantInError) {
				t.Errorf("error should mention %q: %v", c.wantInError, err)
			}
		})
	}
}

// TestRunPrintsRunToStorage pins that the CLI's printed output for macros and
// ensemble configs is exactly RunToStorage's result, formatted.
func TestRunPrintsRunToStorage(t *testing.T) {
	t.Run("macros", func(t *testing.T) {
		printed := captureStdout(t, func() {
			Run(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"), &SocketConfig{})
		})
		result, err := RunToStorage(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		want := captureStdout(t, func() { printStorage(result.Storage) })
		if printed != want || printed == "" {
			t.Errorf("Run's macro output differs from RunToStorage printed "+
				"(%d vs %d bytes)", len(printed), len(want))
		}
	})
	t.Run("ensemble", func(t *testing.T) {
		printed := captureStdout(t, func() {
			Run(LoadApiRunConfigFromYaml("../../cfg/example_ensemble_config.yaml"), &SocketConfig{})
		})
		result, err := RunToStorage(LoadApiRunConfigFromYaml("../../cfg/example_ensemble_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		want := captureStdout(t, func() { printEnsemble(result.Members) })
		if printed != want || printed == "" {
			t.Errorf("Run's ensemble output differs from RunToStorage printed "+
				"(%d vs %d bytes)", len(printed), len(want))
		}
	})
}

// TestRunOutputIsDeterministic pins that the CLI prints partitions in a fixed
// order: storage names come from a map, so unsorted printing varied run to run.
func TestRunOutputIsDeterministic(t *testing.T) {
	first := captureStdout(t, func() {
		Run(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"), &SocketConfig{})
	})
	// Unsorted, the order changes in roughly a quarter of runs (measured), so 50
	// repetitions miss an ordering regression with probability ~1e-6.
	for i := range 50 {
		again := captureStdout(t, func() {
			Run(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"), &SocketConfig{})
		})
		if again != first {
			t.Fatalf("run %d printed different output from run 0", i+1)
		}
	}
	// name order: data_stream, rolling_mean, rolling_var
	lines := strings.Split(strings.TrimSpace(first), "\n")
	order := []string{}
	for _, line := range lines {
		name := strings.Fields(line)[1]
		if len(order) == 0 || order[len(order)-1] != name {
			order = append(order, name)
		}
	}
	if strings.Join(order, ",") != "data_stream,rolling_mean,rolling_var" {
		t.Errorf("partitions printed in order %v, want name order", order)
	}
}
