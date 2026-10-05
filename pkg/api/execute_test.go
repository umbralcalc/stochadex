package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executeMain is a minimal runnable batch config; cases vary one thing each.
const executeMain = `main:
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

func executeWithOutput(output string) string {
	return strings.Replace(executeMain, "OUTPUT", output, 1)
}

// TestExecute drives the CLI's entry point in-process over one input per failure
// class, asserting the classified kind and exit code. test/exit_codes_test.go
// runs the same classes through the built binary.
func TestExecute(t *testing.T) {
	dir := t.TempDir()
	write := func(name, contents string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	malformedCSV := write("malformed.csv", "0,1.0\n1,not-a-number\n")
	macroOn := func(data, partition string) string {
		return data + fmt.Sprintf(`macros:
- type: vector_mean
  name: m
  data: {partition_name: %s}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [10.0]}
  window: 3
`, partition)
	}
	subSim := `data:
  steps: 10
  partitions:
  - name: d
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 3
    seed: 1
`
	csvOf := func(path string) string {
		return fmt.Sprintf("data:\n  source:\n    csv: {path: %q, time_column: 0, state_columns: {d: [1]}}\n", path)
	}
	nilOutput := executeWithOutput("{type: nil}")

	cases := []struct {
		name   string
		config string   // written and passed as --config unless args is set
		socket string   // written and passed as --socket when non-empty
		args   []string // raw args (after the program name) when set
		kind   ErrorKind
	}{
		{name: "a valid batch run", config: nilOutput},
		{name: "a valid macros run", config: macroOn(subSim, "d")},
		{name: "a valid ensemble run", config: nilOutput + "run: {mode: ensemble, seeds: [1, 2]}\n"},

		{name: "no --config", args: []string{}, kind: ErrUsage},

		{name: "a missing config file",
			args: []string{"--config", filepath.Join(dir, "absent.yaml")}, kind: ErrConfig},
		{name: "invalid YAML", config: "main: [unclosed\n", kind: ErrConfig},
		{name: "a key nothing reads",
			config: strings.Replace(nilOutput, "    seed: 1\n", "    seed: 1\n    state_width: 1\n", 1),
			kind:   ErrConfig},
		{name: "a partition with no iteration",
			config: strings.Replace(nilOutput, "    iteration: {type: wiener_process}\n", "", 1),
			kind:   ErrConfig},
		{name: "an upstream naming a missing partition",
			config: strings.Replace(nilOutput, "    seed: 1\n",
				"    seed: 1\n    params_from_upstream: {variances: {upstream: ghost}}\n", 1),
			kind: ErrConfig},
		{name: "an unknown run mode", config: nilOutput + "run: {mode: sweep}\n", kind: ErrConfig},
		{name: "a macro referencing a missing partition", config: macroOn(subSim, "ghost"), kind: ErrConfig},
		{name: "a missing socket config file", config: nilOutput,
			args: nil, socket: "\x00missing", kind: ErrConfig},
		{name: "an invalid socket config", config: nilOutput, socket: "address: [unclosed\n", kind: ErrConfig},

		{name: "a malformed input", config: macroOn(csvOf(malformedCSV), "d"), kind: ErrData},

		{name: "a missing input", config: macroOn(csvOf(filepath.Join(dir, "absent.csv")), "d"),
			kind: ErrUnavailable},
		{name: "an output in a missing directory",
			config: executeWithOutput(fmt.Sprintf("{type: json_log, path: %q}",
				filepath.Join(dir, "missing", "run.log"))),
			kind: ErrUnavailable},

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
`, kind: ErrRuntime},
	}

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := []string{"stochadex"}
			if c.args != nil {
				args = append(args, c.args...)
			} else {
				args = append(args, "--config", write(fmt.Sprintf("case-%d.yaml", i), c.config))
				switch {
				case c.socket == "\x00missing":
					args = append(args, "--socket", filepath.Join(dir, "absent-socket.yaml"))
				case c.socket != "":
					args = append(args, "--socket", write(fmt.Sprintf("socket-%d.yaml", i), c.socket))
				}
			}
			var err error
			captureStdout(t, func() { err = Execute(args) })
			if got := KindOf(err); got != c.kind {
				t.Fatalf("kind %d, want %d (err: %v)", got, c.kind, err)
			}
			want := map[ErrorKind]int{0: 0, ErrUsage: 64, ErrData: 65, ErrRuntime: 70,
				ErrUnavailable: 75, ErrConfig: 78}[c.kind]
			if got := ExitCode(err); got != want {
				t.Errorf("exit code %d, want %d", got, want)
			}
		})
	}
}

// TestLoadConfigErrors pins that LoadConfig returns (not panics) an ErrConfig
// error, while LoadApiRunConfigFromYaml still panics with the underlying error.
func TestLoadConfigErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("main: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if config != nil || KindOf(err) != ErrConfig {
		t.Fatalf("LoadConfig: got (%v, %v), want (nil, an ErrConfig error)", config, err)
	}
	if !didPanic(func() { LoadApiRunConfigFromYaml(path) }) {
		t.Error("LoadApiRunConfigFromYaml should still panic on an invalid config")
	}
}
