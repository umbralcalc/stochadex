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

// batchConfigYAML is a seeded two-partition batch run whose one output view writes
// what condition %s selects to a json_log sink at %s, so the sink path and the
// storage path can be compared.
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
    termination_condition: {type: number_of_steps, max_steps: 30}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: output, condition: %s, function: {type: json_log, path: "%s"}}
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
		Run(config)
		fromSink, err := analysis.NewStateTimeStorageFromJsonLogEntries(logPath)
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "storage vs json_log sink", result.Storage, fromSink)
		if got := len(result.Storage.GetTimes()); got != 31 {
			t.Errorf("expected the initial state plus 30 steps (31 rows), got %d", got)
		}
	})

	t.Run("a macros config returns its expansion's storage", func(t *testing.T) {
		result, err := RunToStorage(mustLoad(t, "../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		expanded, err := Expand(mustLoad(t, "../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		reference, err := RunToStorage(expanded)
		if err != nil {
			t.Fatal(err)
		}
		if result.Members != nil {
			t.Fatal("a macros run should set Storage only")
		}
		assertStoragesEqual(t, "macros config vs its expansion", result.Storage, reference.Storage)
	})

	t.Run("an ensemble config returns members aligned to run.seeds", func(t *testing.T) {
		result, err := RunToStorage(mustLoad(t, "../../cfg/example_ensemble_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		config := mustLoad(t, "../../cfg/example_ensemble_config.yaml")
		reference, err := ensembleRuns(config, config.GetConfigGenerator().GetSimulation(), false)
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
		{"ensemble with no seeds",
			strings.Replace(fmt.Sprintf(batchConfigYAML, "{type: every_step}", scratchLog+"-{member}"),
				"main:", "run: {mode: ensemble}\nmain:", 1),
			"run.seeds"},
		{"an upstream naming a missing partition",
			strings.Replace(fmt.Sprintf(batchConfigYAML, "{type: every_step}", scratchLog),
				"    seed: 2939\n", "    seed: 2939\n    params_from_upstream: {variances: {upstream: ghost}}\n", 1),
			"ghost"},
		{"a within-step dependency cycle",
			`main:
  partitions:
  - {name: a, iteration: {type: copy_values}, params: {partitions: [1], partition_state_values: [0]}, params_from_upstream: {partition_state_values: {upstream: b}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: b, iteration: {type: copy_values}, params: {partitions: [0], partition_state_values: [0]}, params_from_upstream: {partition_state_values: {upstream: a}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: output, condition: {type: every_step}, function: {type: nil}}
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

// rowLines formats a storage's rows as a stdout view writes them — "<time>
// <partition> [values]", led by prefix when set — in time order, partitions in
// name order within a time. It is the reference the CLI's printed output is
// checked against: an independent formatting of RunToStorage's result.
func rowLines(storage *simulator.StateTimeStorage, prefix string) []string {
	names := storage.GetNames()
	sort.Strings(names)
	lines := []string{}
	for step, time := range storage.GetTimes() {
		for _, name := range names {
			values := storage.GetValues(name)
			if step >= len(values) {
				continue
			}
			line := fmt.Sprintf("%v %s %v", time, name, values[step])
			if prefix != "" {
				line = prefix + " " + line
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// TestRunPrintsRunToStorage pins that the CLI's printed output for macros and
// ensemble configs is exactly RunToStorage's result, written through the
// config's default stdout view.
func TestRunPrintsRunToStorage(t *testing.T) {
	t.Run("macros: the result, row by row in time order", func(t *testing.T) {
		printed := captureStdout(t, func() {
			Run(mustLoad(t, "../../cfg/example_macro_config.yaml"))
		})
		result, err := RunToStorage(mustLoad(t, "../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Join(rowLines(result.Storage, ""), "\n") + "\n"
		if printed != want || len(want) < 1000 {
			t.Errorf("Run's macro output differs from RunToStorage's rows "+
				"(%d vs %d bytes)", len(printed), len(want))
		}
	})
	t.Run("ensemble: each member's rows, prefixed, in its own time order", func(t *testing.T) {
		printed := captureStdout(t, func() {
			Run(mustLoad(t, "../../cfg/example_ensemble_config.yaml"))
		})
		result, err := RunToStorage(mustLoad(t, "../../cfg/example_ensemble_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		// Members run concurrently, so their lines interleave; each member's own
		// lines keep its order.
		byMember := map[string][]string{}
		for _, line := range strings.Split(strings.TrimSpace(printed), "\n") {
			fields := strings.Fields(line)
			byMember[fields[0]+" "+fields[1]] = append(byMember[fields[0]+" "+fields[1]], line)
		}
		if len(byMember) != len(result.Members) || len(result.Members) != 4 {
			t.Fatalf("printed %d members, RunToStorage has %d, want 4", len(byMember), len(result.Members))
		}
		for member, run := range result.Members {
			prefix := fmt.Sprintf("member=%d seed=%d", member, run.Seed)
			want := rowLines(run.Storage, prefix)
			if strings.Join(byMember[prefix], "\n") != strings.Join(want, "\n") || len(want) != 21 {
				t.Errorf("%s printed %d lines, want its 21 rows in order:\n%v\nvs\n%v",
					prefix, len(byMember[prefix]), byMember[prefix], want)
			}
		}
	})
}

// TestRunOutputIsDeterministic pins that the CLI prints a macros result in a
// fixed order: time order, partitions in name order within a time. Storage names
// come from a map, so unsorted printing varied run to run.
func TestRunOutputIsDeterministic(t *testing.T) {
	first := captureStdout(t, func() {
		Run(mustLoad(t, "../../cfg/example_macro_config.yaml"))
	})
	// Unsorted, the order changes in roughly a quarter of runs (measured), so 50
	// repetitions miss an ordering regression with probability ~1e-6.
	for i := range 50 {
		again := captureStdout(t, func() {
			Run(mustLoad(t, "../../cfg/example_macro_config.yaml"))
		})
		if again != first {
			t.Fatalf("run %d printed different output from run 0", i+1)
		}
	}
	lines := strings.Split(strings.TrimSpace(first), "\n")
	for i, want := range []string{"data_stream", "rolling_mean", "rolling_var", "data_stream"} {
		if name := strings.Fields(lines[i])[1]; name != want {
			t.Errorf("line %d is partition %s, want %s (time order, names in order)", i, name, want)
		}
	}
}

// runToStorage runs a config as RunToStorage does and returns its storage.
func runToStorage(config *ApiRunConfig) (*simulator.StateTimeStorage, error) {
	result, err := RunToStorage(config)
	if err != nil {
		return nil, err
	}
	return result.Storage, nil
}
