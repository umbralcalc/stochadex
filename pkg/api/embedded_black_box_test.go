package api

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// blackBoxYAML is an outer run whose host partition embeds three inner
// partitions of different widths. Each inner partition is a Wiener process with
// zero variance, so its state stays at its initial values and the host row's
// layout can be checked value for value.
const blackBoxYAML = `main:
  partitions:
  - {name: host, params: {burn_in_steps: [0]}, init_state_values: [0.0, 0.0, 0.0, 0.0, 0.0, 0.0], state_history_depth: 1, seed: 0}
  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [0.0], state_history_depth: 1, seed: 2}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: host
  partitions:
  - {name: one, iteration: {type: wiener_process}, params: {variances: [0.0]}, init_state_values: [1.5], state_history_depth: 1, seed: 1}
  - {name: three, iteration: {type: wiener_process}, params: {variances: [0.0, 0.0, 0.0]}, init_state_values: [2.5, 3.5, 4.5], state_history_depth: 1, seed: 2}
  - {name: two, iteration: {type: wiener_process}, params: {variances: [0.0, 0.0]}, init_state_values: [5.5, 6.5], state_history_depth: 1, seed: 3}
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

func TestEmbeddedRunsAreBlackBoxes(t *testing.T) {
	t.Run("an embedded run that declares no output runs, writes nothing and is recorded by its host row", func(t *testing.T) {
		// This used to crash: an embedded run had to declare output_function: {type: nil}.
		dir := t.TempDir()
		err, printed := executeIn(t, dir, "--config", writeConfigPath(t, blackBoxYAML))
		if err != nil {
			t.Fatal(err)
		}
		if files := filesUnder(t, dir); len(files) != 0 {
			t.Errorf("the run wrote %v", files)
		}
		hostRows := 0
		for _, line := range strings.Split(strings.TrimSpace(printed), "\n") {
			if strings.Contains(line, " host ") {
				hostRows++
			}
		}
		if hostRows != 6 || strings.Contains(printed, " one ") || strings.Contains(printed, " three ") {
			t.Errorf("want the host's 6 rows and no inner rows, printed:\n%s", printed)
		}
	})

	t.Run("the manifest's layout splits every host row into its inner partitions' final states", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, blackBoxYAML))
		if err != nil {
			t.Fatal(err)
		}
		layout := Manifest(config).Embedded
		if len(layout) != 1 || layout[0].Partition != "host" {
			t.Fatalf("layout %+v", layout)
		}
		finals := map[string][]float64{"one": {1.5}, "three": {2.5, 3.5, 4.5}, "two": {5.5, 6.5}}
		result, err := RunToStorage(writeConfig(t, blackBoxYAML))
		if err != nil {
			t.Fatal(err)
		}
		rows := result.Storage.GetValues("host")
		// Row 0 is the host's initial state; every outer step after it is the black box.
		for step, row := range rows[1:] {
			covered := 0
			for _, columns := range layout[0].Columns {
				got := row[columns.Offset : columns.Offset+columns.Width]
				if !floats.Equal(got, finals[columns.Partition]) {
					t.Fatalf("step %d: columns %+v hold %v, want %s's final state %v",
						step+1, columns, got, columns.Partition, finals[columns.Partition])
				}
				covered += columns.Width
			}
			if covered != len(row) {
				t.Fatalf("the layout covers %d of the host row's %d columns", covered, len(row))
			}
		}
	})

	t.Run("the shipped inference example's layout is as wide as its host partition", func(t *testing.T) {
		t.Chdir(repoRootPath(t))
		config, err := LoadConfig("cfg/example_inference_config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		layout := Manifest(config).Embedded[0]
		width := 0
		for _, columns := range layout.Columns {
			width += columns.Width
		}
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		if rows := result.Storage.GetValues(layout.Partition); len(rows) == 0 || len(rows[1]) != width {
			t.Errorf("the layout of %s is %d wide; its rows are %d", layout.Partition, width, len(rows[1]))
		}
	})

	t.Run("a debug log is opt-in, needs no edit, is listed apart and has no provenance", func(t *testing.T) {
		// --debug-embedded adds the embedded run's output, which it does not declare.
		pinnedBuild(t, "pinned")
		dir := t.TempDir()
		debug, log := filepath.Join(dir, "inner.log"), filepath.Join(dir, "run.log")
		yaml := blackBoxYAML + fmt.Sprintf("outputs:\n- {name: log, function: {type: json_log, path: %q}}\n", log)
		config := writeConfigPath(t, yaml)
		debugOn := []string{"--debug-embedded", "host=" + debug}

		option, err := debugEmbeddedOption("host=" + debug)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadConfig(config, option)
		if err != nil {
			t.Fatal(err)
		}
		manifest := Manifest(loaded)
		if len(manifest.Debug) != 1 || manifest.Debug[0].Location != debug ||
			slices.ContainsFunc(manifest.Outputs, func(o ManifestOutput) bool { return o.Location == debug }) {
			t.Fatalf("the debug log should be listed under debug only: %+v / %+v", manifest.Outputs, manifest.Debug)
		}

		if err := Execute(append([]string{"stochadex", "--config", config, "--skip-if-unchanged"}, debugOn...)); err != nil {
			t.Fatal(err)
		}
		// 5 outer steps, each an inner run of 5 rows for 3 partitions. Inner
		// times repeat across outer steps: a debug log is every inner run, in turn.
		if lines := strings.Count(readAll(t, debug), "\n"); lines != 5*5*3 {
			t.Errorf("the debug log has %d entries, want 75", lines)
		}
		if _, err := os.Stat(debug + sidecarSuffix); err == nil {
			t.Error("a debug log was given provenance")
		}
		// The debug log is not a result: a run is skipped without it.
		os.Remove(debug)
		writeFileAt(t, log, "marker\n")
		if err := Execute(append([]string{"stochadex", "--config", config, "--skip-if-unchanged"}, debugOn...)); err != nil {
			t.Fatal(err)
		}
		if readAll(t, log) != "marker\n" {
			t.Error("a missing debug log made the run run again")
		}
	})
}

func TestDebugEmbeddedErrors(t *testing.T) {
	declared := strings.Replace(blackBoxYAML, "  simulation:\n    termination_condition: {type: number_of_steps, max_steps: 4}",
		"  simulation:\n    output_function: {type: nil}\n    termination_condition: {type: number_of_steps, max_steps: 4}", 1)
	cases := []struct {
		name, yaml, spec string
		kind             ErrorKind
		want             string
	}{
		{"an unknown embedded run", blackBoxYAML, "nope=x.log", ErrConfig,
			`--debug-embedded nope=x.log: embedded has no entry named "nope"`},
		{"a run that already declares its output", declared, "host=x.log", ErrConfig,
			"--debug-embedded host=x.log: embedded[name=host].simulation.output_function is already set; change it with --set"},
		{"no path", blackBoxYAML, "host=", ErrUsage, "expected NAME=PATH"},
		{"no name", blackBoxYAML, "=x.log", ErrUsage, "expected NAME=PATH"},
		{"no equals sign", blackBoxYAML, "host", ErrUsage, "expected NAME=PATH"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Execute([]string{"stochadex", "--config", writeConfigPath(t, c.yaml), "--debug-embedded", c.spec})
			if KindOf(err) != c.kind || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected %v containing %q, got %v", c.kind, c.want, err)
			}
		})
	}
	t.Run("a path with YAML specials is taken as written", func(t *testing.T) {
		dir := t.TempDir()
		debug := filepath.Join(dir, "a: b #c.log")
		if err := Execute([]string{"stochadex", "--config", writeConfigPath(t, blackBoxYAML),
			"--debug-embedded", "host=" + debug}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(debug); err != nil {
			t.Errorf("the debug log was not written at its path: %v", err)
		}
	})
}
