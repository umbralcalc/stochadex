package api

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// replaceOnce replaces old with new exactly once, failing if old is absent so a
// test cannot silently run against an unmodified config.
func replaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	if !strings.Contains(s, old) {
		t.Fatalf("test config edit not applied: %q not found", old)
	}
	return strings.Replace(s, old, new, 1)
}

func writeFile(t testing.TB, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFailureClassificationReachesNestedAndDeferredParts covers the parts of a
// config that are resolved somewhere other than the top-level load.
func TestFailureClassificationReachesNestedAndDeferredParts(t *testing.T) {
	dir := t.TempDir()
	t.Run("an embedded run with an unknown iteration type is a config error at load", func(t *testing.T) {
		path := filepath.Join(dir, "embedded.yaml")
		writeFile(t, path, `main:
  partitions:
  - {name: inner_run, init_state_values: [0.0], state_history_depth: 1, seed: 0, params: {burn_in_steps: [0]}}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: inner_run
  partitions:
  - {name: p, iteration: {type: no_such_process}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: nil}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`)
		_, err := LoadConfig(path)
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "no_such_process") {
			t.Errorf("expected an ErrConfig naming the unknown type, got %v", err)
		}
	})
	t.Run("a data: sub-simulation with an unknown iteration type is a config error when run", func(t *testing.T) {
		// data: partitions are resolved only when the macros tier builds storage.
		path := filepath.Join(dir, "data.yaml")
		writeFile(t, path, replaceOnce(t, macroConfigYAML,
			"iteration: {type: data_generation, likelihood: {type: normal, allow_default_covariance_fallback: true}}",
			"iteration: {type: no_such_process}"))
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("the data: block is resolved at run time, so loading should succeed: %v", err)
		}
		_, err = RunToStorage(config)
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "no_such_process") {
			t.Errorf("expected an ErrConfig naming the unknown type, got %v", err)
		}
	})
	t.Run("a misspelled simulation component is a config error at load", func(t *testing.T) {
		path := filepath.Join(dir, "typo.yaml")
		writeFile(t, path, replaceOnce(t, executeWithOutput("{type: nil}"),
			"{type: number_of_steps, max_steps: 5}", "{type: number_of_stpes, max_steps: 5}"))
		_, err := LoadConfig(path)
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "number_of_stpes") {
			t.Errorf("expected an ErrConfig naming the misspelled type, got %v", err)
		}
	})
	t.Run("RunMacros on a config with no macros is a config error", func(t *testing.T) {
		_, err := RunMacros(&ApiRunConfig{})
		if KindOf(err) != ErrConfig {
			t.Errorf("expected an ErrConfig, got %v", err)
		}
	})
	t.Run("a socket file without an address runs without serving", func(t *testing.T) {
		configPath := filepath.Join(dir, "plain.yaml")
		socketPath := filepath.Join(dir, "socket.yaml")
		writeFile(t, configPath, executeWithOutput("{type: nil}"))
		writeFile(t, socketPath, "handle: /handle\nmillisecond_delay: 0\n")
		var err error
		captureStdout(t, func() {
			err = Execute([]string{"stochadex", "--config", configPath, "--socket", socketPath})
		})
		if err != nil {
			t.Errorf("an inactive socket config should run normally, got %v", err)
		}
	})
}

// TestArgParseExitsOnBadArgs re-runs this test binary as a child that calls
// ArgParse with no --config, and checks the child exits with the usage code and
// prints the usage text. ArgParse used to print usage and carry on with an empty
// config path.
func TestArgParseExitsOnBadArgs(t *testing.T) {
	if os.Getenv("STOCHADEX_ARGPARSE_CHILD") == "1" {
		os.Args = []string{"stochadex"}
		ArgParse()
		return // reaching here means ArgParse did not exit
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestArgParseExitsOnBadArgs$")
	cmd.Env = append(os.Environ(), "STOCHADEX_ARGPARSE_CHILD=1")
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != ExitUsage {
		t.Fatalf("expected the child to exit %d, got %v\n%s", ExitUsage, err, output)
	}
	if !strings.Contains(strings.ToLower(string(output)), "usage") {
		t.Errorf("expected usage text, got:\n%s", output)
	}
}
