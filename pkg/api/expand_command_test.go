package api

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	yaml3 "gopkg.in/yaml.v3"
)

// runExpand runs `stochadex expand` with args, returning what it printed to
// stdout and its error.
func runExpand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var err error
	var out string
	captureStderr(t, func() {
		out = captureStdout(t, func() {
			err = Execute(append([]string{"stochadex", "expand"}, args...))
		})
	})
	return out, err
}

func TestExpandCommand(t *testing.T) {
	root := repoRootPath(t)
	// The configs name their inputs from the repository's root.
	t.Chdir(root)
	configs := oracleConfigs(t)
	if len(configs) < 9 {
		t.Fatalf("found %d macros: configs, want every shipped one: %v", len(configs), configs)
	}
	for _, path := range configs {
		t.Run(path+" prints its expansion, which runs as the config does", func(t *testing.T) {
			config := filepath.Join(root, path)
			out, err := runExpand(t, "-c", config)
			if err != nil {
				t.Fatal(err)
			}
			again, err := runExpand(t, "-c", config)
			if err != nil {
				t.Fatal(err)
			}
			if again != out {
				t.Error("two expansions of the same config printed different text")
			}
			// Reference: the expansion made through the library.
			loaded, err := LoadConfig(config)
			if err != nil {
				t.Fatal(err)
			}
			want, err := ExpandToYAML(loaded)
			if err != nil {
				t.Fatal(err)
			}
			if out != string(want) {
				t.Errorf("printed\n%s\nwant\n%s", out, want)
			}
			// The printed text is a config of its own, which runs as the config.
			printed, err := RunToStorage(writeConfig(t, out))
			if err != nil {
				t.Fatalf("the printed expansion does not run: %v", err)
			}
			original, err := RunToStorage(loaded)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := digestOf(printed.Storage), digestOf(original.Storage); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("the printed expansion runs differently from the config:\n%s", digestDiff(got, want))
			}
		})
	}

	t.Run("applies --set overrides before expanding", func(t *testing.T) {
		out, err := runExpand(t, "-c", filepath.Join(root, "cfg", "example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		// The stream's history is as deep as the windows reading it.
		if !strings.Contains(out, "iteration: {type: from_input, input: data}\n      state_history_depth: 150") {
			t.Fatalf("expected the windows of 150 to set the stream's depth:\n%s", out)
		}
		out, err = runExpand(t, "-c", filepath.Join(root, "cfg", "example_macro_config.yaml"),
			"--set", "macros[name=rolling_mean].window=200", "--set", "macros[name=rolling_var].window=200")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "iteration: {type: from_input, input: data}\n      state_history_depth: 200") {
			t.Errorf("the overridden windows did not reach the expansion:\n%s", out)
		}
	})

	t.Run("a config with no macros is a config error, printing nothing", func(t *testing.T) {
		out, err := runExpand(t, "-c", filepath.Join(root, "cfg", "example_config.yaml"))
		if KindOf(err) != ErrConfig || ExitCode(err) != ExitConfig || out != "" {
			t.Errorf("want a config error and nothing printed, got %v and %q", err, out)
		}
	})

	t.Run("a config whose input cannot be read is unavailable", func(t *testing.T) {
		missing := writeConfigPath(t, strings.Replace(readFile(t, filepath.Join(root, "cfg", "example_data_source_config.yaml")),
			"cfg/example_data.csv", "cfg/absent.csv", 1))
		if _, err := runExpand(t, "-c", missing); KindOf(err) != ErrUnavailable {
			t.Errorf("expected ErrUnavailable, got %v", err)
		}
	})

	t.Run("refuses a bad command line", func(t *testing.T) {
		for name, args := range map[string][]string{
			"no config":       {},
			"a run-only flag": {"-c", "x.yaml", "--check"},
		} {
			_, err := runExpand(t, args...)
			var classified *Error
			if !errors.As(err, &classified) || classified.Kind != ErrUsage {
				t.Errorf("%s: want a usage error, got %v", name, err)
			}
		}
	})

	t.Run("printExpansion reports a failed write", func(t *testing.T) {
		loaded, err := LoadConfig(filepath.Join(root, "cfg", "example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := printExpansion(loaded, failingWriter{}); !errors.Is(err, errWriteFailed) {
			t.Errorf("want the write's error, got %v", err)
		}
	})

	t.Run("a document that cannot be written is an error, not partial text", func(t *testing.T) {
		written, err := writeExpansion(expandedDocument{Inputs: &yaml3.Node{Kind: 99}})
		if err == nil || written != nil {
			t.Errorf("want an error and no text, got %v and %q", err, written)
		}
	})
}

// parsed is a node tree read from YAML text.
func parsed(t *testing.T, text string) *yaml3.Node {
	t.Helper()
	var document yaml3.Node
	if err := yaml3.Unmarshal([]byte(text), &document); err != nil {
		t.Fatal(err)
	}
	return document.Content[0]
}

func TestExpansionLayout(t *testing.T) {
	t.Run("a component that fits a line stays on it; one that does not is laid out block by block", func(t *testing.T) {
		node := parsed(t, `{type: embedded_simulation_run, run: {partitions: [{name: a, iteration: {type: normal}, `+
			`params: {values: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24]}}]}}`)
		lay(node)
		written, err := yaml3.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		want := `type: embedded_simulation_run
run:
    partitions:
        - name: a
          iteration: {type: normal}
          params:
            values: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24]
`
		if string(written) != want {
			t.Errorf("laid out as\n%s\nwant\n%s", written, want)
		}
	})

	t.Run("a component exactly as wide as a line stays on it", func(t *testing.T) {
		for width, wantFlow := range map[int]bool{flowWidth: true, flowWidth + 1: false} {
			// {k: {name: x}} is 10 characters around the name.
			node := parsed(t, fmt.Sprintf("{k: {%s: x}}", strings.Repeat("n", width-10)))
			if len(flowText(node)) != width {
				t.Fatalf("built a %d-wide component, want %d", len(flowText(node)), width)
			}
			lay(node)
			if flow := node.Style == yaml3.FlowStyle; flow != wantFlow {
				t.Errorf("width %d: flow %v, want %v", width, flow, wantFlow)
			}
		}
	})

	t.Run("a list of values stays on one line however long; one of collections does not", func(t *testing.T) {
		values := parsed(t, "["+strings.TrimSuffix(strings.Repeat("123456789, ", 20), ", ")+"]")
		lay(values)
		if values.Style != yaml3.FlowStyle {
			t.Error("a long list of values was laid out in block style")
		}
		collections := parsed(t, "["+strings.TrimSuffix(strings.Repeat("{a: 123456789}, ", 10), ", ")+"]")
		lay(collections)
		if collections.Style == yaml3.FlowStyle {
			t.Error("a long list of collections stayed on one line")
		}
	})

	t.Run("a repeated long component is written once, anchored, and aliased after", func(t *testing.T) {
		long := `{type: simulation, horizon: 8, model: {partitions: [{name: price, iteration: {type: expression}}]}}`
		short := `{type: normal}`
		roots := []*yaml3.Node{
			parsed(t, fmt.Sprintf("{type: mcts_apply, environment: %s, likelihood: %s}", long, short)),
			parsed(t, fmt.Sprintf("{type: mcts_tree, environment: %s, likelihood: %s}", long, short)),
			parsed(t, fmt.Sprintf("{type: mcts_rollout, environment: %s}", long)),
		}
		for _, root := range roots {
			lay(root)
		}
		share(roots)
		var text strings.Builder
		for _, root := range roots {
			written, err := yaml3.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			text.Write(written)
		}
		out := text.String()
		if strings.Count(out, "&environment") != 1 || strings.Count(out, "*environment") != 2 ||
			strings.Count(out, "horizon: 8") != 1 {
			t.Errorf("the environment is not written once and aliased twice:\n%s", out)
		}
		if strings.Count(out, "{type: normal}") != 2 {
			t.Errorf("a short repeat was aliased; it fits on its line as it is:\n%s", out)
		}
	})

	t.Run("two different repeated components under one name get their own anchors", func(t *testing.T) {
		first := `{type: simulation, horizon: 8, model: {partitions: [{name: first, iteration: {type: expression}}]}}`
		second := `{type: simulation, horizon: 9, model: {partitions: [{name: other, iteration: {type: expression}}]}}`
		var roots []*yaml3.Node
		for index, environment := range []string{first, first, second, second} {
			// Each root differs, so only its environment repeats.
			roots = append(roots, parsed(t, fmt.Sprintf("{type: mcts_tree, slot: %d, environment: %s}",
				index, environment)))
		}
		for _, root := range roots {
			lay(root)
		}
		share(roots)
		for index, wantAnchor := range []string{"environment", "", "environment_2", ""} {
			environment := mappingValue(roots[index], "environment")
			if environment.Anchor != wantAnchor {
				t.Errorf("root %d: anchor %q, want %q", index, environment.Anchor, wantAnchor)
			}
		}
		if alias := mappingValue(roots[3], "environment"); alias.Kind != yaml3.AliasNode || alias.Value != "environment_2" {
			t.Errorf("the second repeat is not an alias of environment_2: %+v", alias)
		}
	})

	t.Run("a partition's params and wiring are laid out as its iteration is", func(t *testing.T) {
		long := make([]float64, 40)
		partition := expandedPartition{
			Name:               "p",
			Iteration:          component("posterior_mean", nil),
			Params:             map[string][]float64{"particle_loglikes": long, "num_params": {2}},
			ParamsAsPartitions: map[string][]string{"loglike_partitions": {"l"}},
			ParamsFromUpstream: map[string]expandedUpstream{"mean": {Upstream: "m"}},
		}
		written, err := yaml3.Marshal(partition)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"params:\n    num_params: [2]\n    particle_loglikes: [0, 0,",
			"params_as_partitions: {loglike_partitions: [l]}",
			"params_from_upstream: {mean: {upstream: m}}",
		} {
			if !strings.Contains(string(written), want) {
				t.Errorf("want %q in\n%s", want, written)
			}
		}
	})

	t.Run("an anchor's name keeps only what YAML allows", func(t *testing.T) {
		names := map[string]bool{}
		if got := anchorName("plan_tree/root state", names); got != "plan_tree_root_state" {
			t.Errorf("got %q", got)
		}
	})
}
