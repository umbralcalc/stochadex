package api

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// deadlockingConfig is a run whose two partitions read each other within the
// step, which cannot run.
const deadlockingConfig = `main:
  partitions:
  - name: prey
    iteration: {type: wiener_process}
    params_from_upstream:
      variances: {upstream: predator}
    init_state_values: [1.0]
    state_history_depth: 1
    seed: 1
  - name: predator
    iteration: {type: wiener_process}
    params_from_upstream:
      variances: {upstream: prey}
    init_state_values: [1.0]
    state_history_depth: 1
    seed: 2
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 3}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
outputs:
- {name: out, condition: {type: nil}, function: {type: nil}}
`

// runGraph runs `stochadex graph` with args, returning what it printed to
// stdout and its error.
func runGraph(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var err error
	var out string
	captureStderr(t, func() {
		out = captureStdout(t, func() {
			err = Execute(append([]string{"stochadex", "graph"}, args...))
		})
	})
	return out, err
}

func TestGraphCommand(t *testing.T) {
	config := filepath.Join(repoDir, "cfg", "example_composition_config.yaml")

	t.Run("prints the main run's graph as Mermaid by default", func(t *testing.T) {
		out, err := runGraph(t, "-c", config)
		if err != nil {
			t.Fatal(err)
		}
		// Reference: the graph rendered from the same config through the library.
		loaded, err := LoadConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		want := new(strings.Builder)
		if err := printGraph(loaded, "mermaid", want); err != nil {
			t.Fatal(err)
		}
		if out != want.String() {
			t.Errorf("got\n%s\nwant\n%s", out, want)
		}
		for _, part := range []string{"flowchart", "data_stream", "|latest_data_values|"} {
			if !strings.Contains(out, part) {
				t.Errorf("the Mermaid graph has no %q:\n%s", part, out)
			}
		}
	})

	t.Run("prints DOT with --format dot", func(t *testing.T) {
		out, err := runGraph(t, "-c", config, "--format", "dot")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(out, "digraph simulation {") ||
			!strings.Contains(out, `label="latest_data_values"`) {
			t.Errorf("not the DOT graph:\n%s", out)
		}
	})

	t.Run("applies --set overrides before drawing", func(t *testing.T) {
		// Pointed at itself, rolling_mean's upstream read becomes a self-loop.
		out, err := runGraph(t, "-c", config, "--set",
			"main.partitions[name=rolling_mean].params_from_upstream.latest_data_values.upstream=rolling_mean")
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "[rolling_mean]") {
			t.Errorf("want the self-loop reported, got %v", err)
		}
		if !strings.Contains(out, "n1 -->|latest_data_values| n1") {
			t.Errorf("the override was not drawn:\n%s", out)
		}
	})

	t.Run("prints the graph, then fails, on a deadlock cycle", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "deadlock.yaml")
		writeFile(t, path, deadlockingConfig)
		out, err := runGraph(t, "-c", path)
		if err == nil {
			t.Fatal("want an error for a within-step cycle")
		}
		if ExitCode(err) != ExitConfig {
			t.Errorf("want exit %d, got %d", ExitConfig, ExitCode(err))
		}
		if !strings.Contains(err.Error(), "deadlock") ||
			!strings.Contains(err.Error(), "[prey predator]") {
			t.Errorf("the error does not name the cycle: %v", err)
		}
		if !strings.Contains(out, "class n0 deadlock;") || !strings.Contains(out, "class n1 deadlock;") {
			t.Errorf("the graph does not mark the cycle:\n%s", out)
		}
	})

	t.Run("refuses a bad command line", func(t *testing.T) {
		for name, args := range map[string][]string{
			"no config":       {},
			"unknown format":  {"-c", config, "--format", "svg"},
			"a run-only flag": {"-c", config, "--seed-range", "1:2"},
		} {
			_, err := runGraph(t, args...)
			var classified *Error
			if !errors.As(err, &classified) || classified.Kind != ErrUsage {
				t.Errorf("%s: want a usage error, got %v", name, err)
			}
		}
	})

	t.Run("a missing config is a config error", func(t *testing.T) {
		_, err := runGraph(t, "-c", filepath.Join(t.TempDir(), "missing.yaml"))
		if KindOf(err) != ErrConfig {
			t.Errorf("got %v", err)
		}
	})

	t.Run("printGraph refuses an unknown format and reports a failed write", func(t *testing.T) {
		loaded, err := LoadConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		if err := printGraph(loaded, "svg", &out); KindOf(err) != ErrUsage || out.Len() != 0 {
			t.Errorf("want a usage error and nothing written, got %v and %q", err, out.String())
		}
		if err := printGraph(loaded, "dot", failingWriter{}); !errors.Is(err, errWriteFailed) {
			t.Errorf("want the write's error, got %v", err)
		}
	})
}

// errWriteFailed is what failingWriter fails with.
var errWriteFailed = errors.New("write failed")

// failingWriter is a writer whose every write fails.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func TestGraphCommandOnAMacrosConfig(t *testing.T) {
	config := filepath.Join(repoDir, "cfg", "example_macro_config.yaml")
	out, err := runGraph(t, "-c", config)
	if err != nil {
		t.Fatal(err)
	}
	// Reference: the graph of the config's expansion, rendered through the
	// library.
	loaded, err := LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := Expand(loaded)
	if err != nil {
		t.Fatal(err)
	}
	want := new(strings.Builder)
	if err := printGraph(expanded, "mermaid", want); err != nil {
		t.Fatal(err)
	}
	if out != want.String() {
		t.Errorf("printed\n%s\nwant the expansion's graph\n%s", out, want)
	}
	// The variance reads the mean it is centred on within the step.
	id := func(name string) string {
		t.Helper()
		match := regexp.MustCompile(`(n\d+)\["` + name + `"\]`).FindStringSubmatch(out)
		if match == nil {
			t.Fatalf("no node %q in\n%s", name, out)
		}
		return match[1]
	}
	if edge := id("rolling_mean") + " -->|mean| " + id("rolling_var"); !strings.Contains(out, edge) {
		t.Errorf("no rolling_mean → rolling_var edge (%s) in\n%s", edge, out)
	}

	t.Run("a macros config whose input cannot be read fails, as its run would", func(t *testing.T) {
		missing := writeConfigPath(t, strings.Replace(readFile(t, "../../cfg/example_data_source_config.yaml"),
			"cfg/example_data.csv", "cfg/absent.csv", 1))
		if _, err := runGraph(t, "-c", missing); KindOf(err) != ErrUnavailable {
			t.Errorf("expected ErrUnavailable, got %v", err)
		}
	})
}
