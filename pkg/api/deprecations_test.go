package api

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// pairYAML is a two-partition run whose output is the deprecated main
// simulation pair: a condition and a json_log at path, which may need quoting.
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

// stderrOf runs the CLI and returns what it wrote to stderr.
func stderrOf(t *testing.T, args ...string) string {
	t.Helper()
	var err error
	printed := captureStderr(t, func() {
		captureStdout(t, func() { err = Execute(append([]string{"stochadex"}, args...)) })
	})
	if err != nil {
		t.Fatal(err)
	}
	return printed
}

func TestDeprecationNotices(t *testing.T) {
	t.Run("the output pair's notice gives its replacement, which pasted in runs identically and quietly", func(t *testing.T) {
		dir := t.TempDir()
		// A path that YAML must quote, so the suggested text has to get it right.
		legacy := pairYAML(filepath.Join(dir, "run: one #1.log"))
		config, err := LoadConfig(writeConfigPath(t, legacy))
		if err != nil {
			t.Fatal(err)
		}
		notices := config.Deprecations()
		if len(notices) != 1 {
			t.Fatalf("want one notice, got %v", notices)
		}
		_, replacement, found := strings.Cut(notices[0], "declare the output as a view, ")
		if !found || !strings.HasPrefix(replacement, "outputs: [") {
			t.Fatalf("the notice gives no replacement: %q", notices[0])
		}
		pasted := strings.NewReplacer(
			"    output_condition: {type: only_given_partitions, partitions: [b]}\n", "",
			fmt.Sprintf("    output_function: {type: json_log, path: %q}\n", filepath.Join(dir, "run: one #1.log")), "",
		).Replace(legacy) + replacement + "\n"
		quiet, err := LoadConfig(writeConfigPath(t, pasted))
		if err != nil {
			t.Fatalf("the suggested replacement does not load: %v\n%s", err, pasted)
		}
		if len(quiet.Deprecations()) != 0 {
			t.Errorf("the replacement still carries notices: %v", quiet.Deprecations())
		}
		before, err := RunWith(config, CaptureView("all", nil), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		written := keyedEntries(t, filepath.Join(dir, "run: one #1.log"))
		after, err := RunWith(quiet, CaptureView("all", nil), WithConfigOutputs())
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "pasted vs pair", after.Views["all"], before.Views["all"])
		assertSameEntries(t, "the log", keyedEntries(t, filepath.Join(dir, "run: one #1.log")), written)
		if len(written) != 21 {
			t.Errorf("the log has %d entries, want b's 21 only", len(written))
		}
	})

	t.Run("a pair with only a function suggests a view with no condition", func(t *testing.T) {
		yaml := strings.Replace(pairYAML(filepath.Join(t.TempDir(), "x.log")),
			"    output_condition: {type: only_given_partitions, partitions: [b]}\n", "", 1)
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		if notice := config.Deprecations()[0]; strings.Contains(notice, "condition:") ||
			!strings.Contains(notice, "outputs: [{name: output, function: {type: json_log, path: ") {
			t.Errorf("notice %q", notice)
		}
	})

	t.Run("a pair with only a condition suggests stdout, the function it defaults to", func(t *testing.T) {
		yaml := strings.Replace(pairYAML("x.log"), "    output_function: {type: json_log, path: \"x.log\"}\n", "", 1)
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		want := "outputs: [{name: output, condition: {type: only_given_partitions, partitions: [b]}, function: {type: stdout}}]"
		if notice := config.Deprecations()[0]; !strings.HasSuffix(notice, want) {
			t.Errorf("notice %q, want it to end %q", notice, want)
		}
	})

	t.Run("the CLI prints each notice once, even when an ensemble reloads its members", func(t *testing.T) {
		dir := t.TempDir()
		legacy := strings.Replace(pairYAML(filepath.Join(dir, "run-{member}.log")), "seed: 4}", "seed: 4}", 1) +
			"run: {mode: ensemble, seeds: [5, 6, 7]}\n"
		printed := stderrOf(t, "--config", writeConfigPath(t, legacy))
		if count := strings.Count(printed, "is deprecated"); count != 1 {
			t.Errorf("printed %d notices, want 1:\n%s", count, printed)
		}
	})

	t.Run("data: prints its notice once, naming the input it stands for", func(t *testing.T) {
		yaml := inputsAsData(t, readFile(t, filepath.Join(repoRootPath(t), "cfg", "example_macro_config.yaml")))
		printed := stderrOf(t, "--config", writeConfigPath(t, yaml))
		want := "stochadex: data: is deprecated: it is shorthand for one input, so declare it under " +
			"inputs: as inputs: {data: {simulation: ...}}"
		if strings.Count(printed, "is deprecated") != 1 || !strings.Contains(printed, want) {
			t.Errorf("stderr:\n%s", printed)
		}
		source := "data:\n  source: {csv: {path: x.csv, time_column: 0, state_columns: {s: [1]}}}\n" +
			"macros:\n- {type: vector_mean, name: m, data: {partition_name: s}, kernel: {type: exponential}, " +
			"params: {exponential_weighting_timescale: [2.0]}, window: 3}\n"
		config, err := LoadConfig(writeConfigPath(t, source))
		if err != nil {
			t.Fatal(err)
		}
		if notices := config.Deprecations(); len(notices) != 1 || !strings.Contains(notices[0], "inputs: {data: {source: ...}}") {
			t.Errorf("a source data: block's notice: %v", notices)
		}
	})

	t.Run("the library records notices but prints nothing", func(t *testing.T) {
		path := writeConfigPath(t, pairYAML(filepath.Join(t.TempDir(), "x.log")))
		var config *ApiRunConfig
		printed := captureStderr(t, func() {
			var err error
			if config, err = LoadConfig(path); err != nil {
				t.Fatal(err)
			}
		})
		if printed != "" || len(config.Deprecations()) != 1 {
			t.Errorf("printed %q with notices %v", printed, config.Deprecations())
		}
	})

	t.Run("inspect --io lists the notices", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, pairYAML(filepath.Join(t.TempDir(), "x.log"))))
		if err != nil {
			t.Fatal(err)
		}
		if notices := Manifest(config).Deprecations; len(notices) != 1 {
			t.Errorf("manifest deprecations %v", notices)
		}
	})

	t.Run("an embedded run's own output is its debug log, not deprecated", func(t *testing.T) {
		yaml := checkYAML(walkPartition("a", "")+hostPartition, strings.Replace(hostedRun,
			"  simulation:\n", "  simulation:\n    output_condition: {type: every_step}\n    output_function: {type: nil}\n", 1))
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		if notices := config.Deprecations(); len(notices) != 0 {
			t.Errorf("an embedded run's pair was called deprecated: %v", notices)
		}
	})

	t.Run("no shipped config uses a deprecated form", func(t *testing.T) {
		root := repoRootPath(t)
		t.Chdir(root)
		paths := []string{}
		for _, pattern := range []string{"cfg/example_*.yaml", "models/*/declarative.yaml",
			".claude/skills/stochadex-model/recipes/*.yaml"} {
			matches, _ := filepath.Glob(pattern)
			paths = append(paths, matches...)
		}
		if len(paths) < 30 {
			t.Fatalf("found only %d configs", len(paths))
		}
		quiet := []string{}
		for _, path := range paths {
			config, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if notices := config.Deprecations(); len(notices) > 0 {
				t.Errorf("%s: %v", path, notices)
			} else {
				quiet = append(quiet, path)
			}
		}
		if !slices.Equal(quiet, paths) {
			t.Errorf("%d of %d shipped configs are quiet", len(quiet), len(paths))
		}
	})
}

func TestRunWithParsedArgsPrintsDeprecations(t *testing.T) {
	path := writeConfigPath(t, pairYAML(filepath.Join(t.TempDir(), "x.log")))
	printed := captureStderr(t, func() {
		captureStdout(t, func() { RunWithParsedArgs(ParsedArgs{ConfigFile: path}) })
	})
	if strings.Count(printed, "is deprecated") != 1 || !strings.Contains(printed, "outputs: [{name: output") {
		t.Errorf("stderr:\n%s", printed)
	}
}

func TestFlowSpec(t *testing.T) {
	spec := simulator.ComponentSpec{Type: "json_log", Fields: map[string]interface{}{
		"path": "out dir/a: b #1.log", "nested": map[interface{}]interface{}{"k": []interface{}{1, 2.5}}}}
	if got, want := flowSpec(spec), `{type: json_log, nested: {k: [1, 2.5]}, path: 'out dir/a: b #1.log'}`; got != want {
		t.Errorf("flowSpec gave %s, want %s", got, want)
	}
}
