package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// validMain is a minimal runnable batch config; cases below vary one thing each.
const validMain = `main:
  partitions:
  - name: w
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 1
  simulation:
    output_condition: {type: every_step}
    output_function: OUTPUT
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`

func withOutput(output string) string {
	return strings.Replace(validMain, "OUTPUT", output, 1)
}

// macroOver is a macros config whose data comes from the given data: block.
func macroOver(data, partition string) string {
	return data + `macros:
- type: vector_mean
  name: m
  data: {partition_name: ` + partition + `}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [10.0]}
  window: 3
`
}

const subSimData = `data:
  steps: 10
  partitions:
  - name: d
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 3
    seed: 1
`

func csvData(path string) string {
	return `data:
  source:
    csv: {path: "` + path + `", time_column: 0, state_columns: {d: [1]}}
`
}

// TestBinaryExitCodes runs the real CLI over one config per failure class and
// checks the exit code an orchestrator would see (BSD sysexits: 64 usage, 65
// data, 70 runtime, 75 unavailable — the only class worth retrying — 78 config),
// and that every failure the engine intercepts is reported as one stderr line,
// never as a Go panic trace.
func TestBinaryExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build+run in -short mode")
	}
	binary := filepath.Join(t.TempDir(), "stochadex")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join("..", "cmd", "stochadex")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI: %v\n%s", err, out)
	}

	dir := t.TempDir()
	malformedCSV := filepath.Join(dir, "malformed.csv")
	if err := os.WriteFile(malformedCSV, []byte("0,1.0\n1,not-a-number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readOnlyDir := filepath.Join(dir, "read-only")
	if err := os.Mkdir(readOnlyDir, 0o500); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		config string // written to a file and passed as --config, unless args is set
		args   []string
		want   int
	}{
		{name: "a valid run succeeds", config: withOutput("{type: nil}"), want: 0},

		{name: "no --config argument", args: []string{}, want: 64},

		{name: "the config file does not exist",
			args: []string{"--config", filepath.Join(dir, "absent.yaml")}, want: 78},
		{name: "invalid YAML", config: "main: [unclosed\n", want: 78},
		{name: "a key nothing reads",
			config: strings.Replace(withOutput("{type: nil}"), "    seed: 1\n",
				"    seed: 1\n    state_width: 1\n", 1), want: 78},
		{name: "an unknown iteration type",
			config: strings.Replace(withOutput("{type: nil}"), "wiener_process", "no_such_process", 1),
			want:   78},
		{name: "a partition with no iteration",
			config: strings.Replace(withOutput("{type: nil}"),
				"    iteration: {type: wiener_process}\n", "", 1), want: 78},
		{name: "an upstream naming a missing partition",
			config: strings.Replace(withOutput("{type: nil}"), "    seed: 1\n",
				"    seed: 1\n    params_from_upstream: {variances: {upstream: ghost}}\n", 1),
			want: 78},
		{name: "a within-step dependency cycle", config: `main:
  partitions:
  - {name: a, iteration: {type: copy_values}, params: {partitions: [1], partition_state_values: [0]}, params_from_upstream: {partition_state_values: {upstream: b}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: b, iteration: {type: copy_values}, params: {partitions: [0], partition_state_values: [0]}, params_from_upstream: {partition_state_values: {upstream: a}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, want: 78},
		{name: "an unknown run mode",
			config: withOutput("{type: nil}") + "run: {mode: sweep}\n", want: 78},
		{name: "a macro referencing a missing partition",
			config: macroOver(subSimData, "ghost"), want: 78},

		{name: "an input file that is malformed",
			config: macroOver(csvData(malformedCSV), "d"), want: 65},

		{name: "an input file that does not exist",
			config: macroOver(csvData(filepath.Join(dir, "absent.csv")), "d"), want: 75},
		{name: "an output path in a missing directory",
			config: withOutput(`{type: json_log, path: "` +
				filepath.Join(dir, "missing", "run.log") + `"}`), want: 75},
		{name: "an output path in a directory that cannot be written",
			config: withOutput(`{type: json_log, path: "` +
				filepath.Join(readOnlyDir, "run.log") + `"}`), want: 75},
		{name: "an unreachable websocket server",
			config: withOutput(`{type: websocket, url: "ws://127.0.0.1:1/never"}`), want: 75},
		{name: "an unreachable database",
			config: withOutput(`{type: postgres, driver: postgres, ` +
				`dsn: "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1", ` +
				`table: results}`), want: 75},

		{name: "the simulation failing while it runs", config: `main:
  partitions:
  - name: replay
    iteration: {type: from_storage, data: [[1.0], [2.0]]}
    init_state_values: [1.0]
    state_history_depth: 1
    seed: 0
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
    execution_strategy: {type: inline}
`, want: 70},
	}

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := c.args
			if args == nil {
				path := filepath.Join(dir, "case"+string(rune('a'+i))+".yaml")
				if err := os.WriteFile(path, []byte(c.config), 0o644); err != nil {
					t.Fatal(err)
				}
				args = []string{"--config", path}
			}
			cmd := exec.Command(binary, args...)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			err := cmd.Run()
			got := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				got = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("running the CLI: %v", err)
			}
			if got != c.want {
				t.Fatalf("exit code %d, want %d\nstderr:\n%s", got, c.want, stderr.String())
			}
			if c.want != 0 {
				if strings.Contains(stderr.String(), "goroutine ") {
					t.Errorf("an intercepted failure should not print a panic trace:\n%s",
						stderr.String())
				}
				if c.want != 64 && !strings.Contains(stderr.String(), "stochadex: ") {
					t.Errorf("expected a one-line \"stochadex: ...\" error on stderr:\n%s",
						stderr.String())
				}
			}
		})
	}
}
