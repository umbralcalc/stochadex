package api

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// pairYAML is a two-partition run whose output is the removed main simulation
// pair: a condition and a json_log at path, which may need quoting.
func pairYAML(path string) string {
	return fmt.Sprintf(`main:
  partitions:
  - {name: a, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 3}
  - {name: b, iteration: {type: wiener_process}, params: {variances: [2.0]}, init_state_values: [1.0], state_history_depth: 1, seed: 4}
  simulation:
    output_condition: {type: only_given_partitions, partitions: [b]}
    output_function: {type: json_log, path: %q}
    termination_condition: {type: number_of_steps, max_steps: 20}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, path)
}

// loadError loads yaml and returns its failure, which must be a config error.
func loadError(t *testing.T, yaml string) string {
	t.Helper()
	_, err := LoadConfig(writeConfigPath(t, yaml))
	if KindOf(err) != ErrConfig {
		t.Fatalf("expected a config error, got %v", err)
	}
	return err.Error()
}

func TestRemovedForms(t *testing.T) {
	t.Run("the output pair's error gives the view it becomes, which pasted in writes what the pair selected", func(t *testing.T) {
		dir := t.TempDir()
		// A path that YAML must quote, so the given text has to get it right.
		path := filepath.Join(dir, "run: one #1.log")
		legacy := pairYAML(path)
		message := loadError(t, legacy)
		_, replacement, found := strings.Cut(message, "declare the output as a view, ")
		if !found || !strings.HasPrefix(replacement, "outputs: [") {
			t.Fatalf("the error gives no replacement: %q", message)
		}
		pasted := strings.NewReplacer(
			"    output_condition: {type: only_given_partitions, partitions: [b]}\n", "",
			fmt.Sprintf("    output_function: {type: json_log, path: %q}\n", path), "",
		).Replace(legacy) + replacement + "\n"
		config, err := LoadConfig(writeConfigPath(t, pasted))
		if err != nil {
			t.Fatalf("the given replacement does not load: %v\n%s", err, pasted)
		}
		result, err := RunWith(config, CaptureView("b", &simulator.OnlyGivenPartitionsOutputCondition{
			Partitions: map[string]bool{"b": true}}), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		written := keyedEntries(t, path)
		if len(written) != 21 {
			t.Errorf("the log has %d entries, want b's 21 only", len(written))
		}
		assertSameEntries(t, "the log", written, storageEntries(result.Views["b"]))
	})

	t.Run("a pair with only a function gives a view with no condition", func(t *testing.T) {
		message := loadError(t, strings.Replace(pairYAML(filepath.Join(t.TempDir(), "x.log")),
			"    output_condition: {type: only_given_partitions, partitions: [b]}\n", "", 1))
		if strings.Contains(message, "condition:") ||
			!strings.Contains(message, "outputs: [{name: output, function: {type: json_log, path: ") {
			t.Errorf("error %q", message)
		}
	})

	t.Run("a pair with only a condition gives stdout, the function it defaulted to", func(t *testing.T) {
		message := loadError(t, strings.Replace(pairYAML("x.log"),
			"    output_function: {type: json_log, path: \"x.log\"}\n", "", 1))
		want := "outputs: [{name: output, condition: {type: only_given_partitions, partitions: [b]}, function: {type: stdout}}]"
		if !strings.HasSuffix(message, want) {
			t.Errorf("error %q, want it to end %q", message, want)
		}
	})

	t.Run("data: names the input it becomes, by its form", func(t *testing.T) {
		simulation := inputsAsData(t, readFile(t, filepath.Join(repoRootPath(t), "cfg", "example_macro_config.yaml")))
		if message := loadError(t, simulation); !strings.Contains(message,
			"data: was removed: it was shorthand for one input, so declare it under inputs: as "+
				"inputs: {data: {simulation: ...}}, its contents moved under simulation:") {
			t.Errorf("a simulation data: block's error: %q", message)
		}
		source := "data:\n  source: {csv: {path: x.csv, time_column: 0, state_columns: {s: [1]}}}\n" +
			"macros:\n- {type: vector_mean, name: m, data: {partition_name: s}, kernel: {type: exponential}, " +
			"params: {exponential_weighting_timescale: [2.0]}, window: 3}\n"
		if message := loadError(t, source); !strings.Contains(message, "inputs: {data: {source: ...}}") {
			t.Errorf("a source data: block's error: %q", message)
		}
		if message := loadError(t, "data: 3\n"); !strings.Contains(message, "{simulation: ...}") {
			t.Errorf("a data: that is not a block: %q", message)
		}
	})

	t.Run("the CLI fails with the error, as a config error", func(t *testing.T) {
		err := Execute([]string{"stochadex", "--config", writeConfigPath(t, pairYAML("x.log"))})
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "outputs: [{name: output") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("invalid YAML reports its own error, not a removed form", func(t *testing.T) {
		if message := loadError(t, "data: [\n"); strings.Contains(message, "removed") {
			t.Errorf("got %q", message)
		}
	})

	t.Run("an embedded run's own output is its debug log, and still loads", func(t *testing.T) {
		yaml := checkYAML(walkPartition("a", "")+hostPartition, strings.Replace(hostedRun,
			"  simulation:\n", "  simulation:\n    output_condition: {type: every_step}\n    output_function: {type: nil}\n", 1))
		if _, err := LoadConfig(writeConfigPath(t, yaml)); err != nil {
			t.Errorf("an embedded run's pair was refused: %v", err)
		}
	})
}

// TestRunWithParsedArgs runs a config through RunWithParsedArgs, and re-runs this
// test binary as a child that gives it a config with a removed form, checking the
// child exits with the config code and prints the error.
func TestRunWithParsedArgs(t *testing.T) {
	if path := os.Getenv("STOCHADEX_RUN_PARSED_CHILD"); path != "" {
		RunWithParsedArgs(ParsedArgs{ConfigFile: path})
		return // reaching here means RunWithParsedArgs did not exit
	}
	t.Run("it runs a config", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.log")
		config := writeConfigPath(t, fmt.Sprintf(batchConfigYAML, "{type: every_step}", path))
		captureStderr(t, func() { RunWithParsedArgs(ParsedArgs{ConfigFile: config}) })
		reference, err := RunToStorage(mustLoad(t, config))
		if err != nil {
			t.Fatal(err)
		}
		assertSameEntries(t, "the log", keyedEntries(t, path), storageEntries(reference.Storage))
	})
	t.Run("it exits with the config code on a config error", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRunWithParsedArgs$")
		cmd.Env = append(os.Environ(), "STOCHADEX_RUN_PARSED_CHILD="+writeConfigPath(t, pairYAML("x.log")))
		output, err := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != ExitConfig {
			t.Fatalf("expected the child to exit %d, got %v\n%s", ExitConfig, err, output)
		}
		if !strings.Contains(string(output), "output_condition / output_function was removed") {
			t.Errorf("expected the error, got:\n%s", output)
		}
	})
}

func TestFlowSpec(t *testing.T) {
	spec := simulator.ComponentSpec{Type: "json_log", Fields: map[string]interface{}{
		"path": "out dir/a: b #1.log", "nested": map[interface{}]interface{}{"k": []interface{}{1, 2.5}}}}
	if got, want := flowSpec(spec), `{type: json_log, nested: {k: [1, 2.5]}, path: 'out dir/a: b #1.log'}`; got != want {
		t.Errorf("flowSpec gave %s, want %s", got, want)
	}
}
