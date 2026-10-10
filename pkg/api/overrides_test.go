package api

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// The oracle for every override is the same config with the value edited into
// the file by hand: an overridden run must equal that run exactly, and differ
// from the unedited one, so the test cannot pass by the override doing nothing.

// runLoaded loads a config file with options and runs it to storage.
func runLoaded(t *testing.T, contents string, options ...LoadOption) *simulator.StateTimeStorage {
	t.Helper()
	config, err := LoadConfig(writeConfigPath(t, contents), options...)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunToStorage(config)
	if err != nil {
		t.Fatal(err)
	}
	return result.Storage
}

// assertEditedRun checks that got equals the run of edited, and that edited's
// run differs from the unedited model's.
func assertEditedRun(t *testing.T, got *simulator.StateTimeStorage, model, edited string) {
	t.Helper()
	want := runLoaded(t, edited)
	assertStoragesEqual(t, "overridden vs edited", got, want)
	if storagesMatch(want, runLoaded(t, model)) {
		t.Fatal("the edit changes nothing, so this case proves nothing")
	}
}

// storagesMatch reports whether two storages hold the same rows.
func storagesMatch(a, b *simulator.StateTimeStorage) bool {
	return reflect.DeepEqual(storageEntries(a), storageEntries(b))
}

func envOf(values map[string]string) LoadOption {
	return WithEnv(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
}

const (
	firstSeed   = "main.partitions[name=first_wiener_process].seed"
	maxSteps    = "main.simulation.termination_condition.max_steps"
	secondParam = "main.partitions[name=second_wiener_process].params.variances"
)

func TestSetOverrides(t *testing.T) {
	model := serveConfigYAML
	cases := []struct {
		name   string
		sets   [][2]string
		edited string
	}{
		{"a partition's seed, selected by name",
			[][2]string{{firstSeed, "99"}},
			replaceOnce(t, model, "seed: 7167", "seed: 99")},
		{"a params list",
			[][2]string{{secondParam, "[4.0]"}},
			replaceOnce(t, model, "variances: [1.0]\n", "variances: [4.0]\n")},
		{"a value in a flow mapping",
			[][2]string{{maxSteps, "12"}},
			replaceOnce(t, model, "max_steps: 40", "max_steps: 12")},
		{"a whole component, replaced as a mapping",
			[][2]string{{"main.simulation.timestep_function", "{type: constant, stepsize: 0.25}"}},
			replaceOnce(t, model, "stepsize: 1.0", "stepsize: 0.25")},
		{"a value under an entry whose name has a dot",
			[][2]string{{"main.partitions[name=second.wiener].seed", "5"}},
			replaceOnce(t, model, "seed: 2939", "seed: 5")},
		{"a value that was null",
			[][2]string{{"main.simulation.init_time_value", "3.0"}},
			replaceOnce(t, model, "init_time_value: 0.0", "init_time_value: 3.0")},
		{"several, applied in order so the last wins",
			[][2]string{{firstSeed, "1"}, {maxSteps, "12"}, {firstSeed, "99"}},
			replaceOnce(t, replaceOnce(t, model, "seed: 7167", "seed: 99"), "max_steps: 40", "max_steps: 12")},
	}
	// The model a case starts from: the null case starts from a null value,
	// and the dotted-name case from a partition named with a dot.
	startFrom := func(name string) string {
		switch {
		case strings.Contains(name, "null"):
			return replaceOnce(t, model, "init_time_value: 0.0", "init_time_value: ~")
		case strings.Contains(name, "dot"):
			return replaceOnce(t, model, "name: second_wiener_process", "name: second.wiener")
		}
		return model
	}
	for _, c := range cases {
		t.Run(c.name+" matches the value edited into the file", func(t *testing.T) {
			options := []LoadOption{}
			for _, set := range c.sets {
				options = append(options, WithSet(set[0], set[1]))
			}
			start := startFrom(c.name)
			edited := c.edited
			if start != model && strings.Contains(c.name, "dot") {
				edited = replaceOnce(t, c.edited, "name: second_wiener_process", "name: second.wiener")
			}
			assertEditedRun(t, runLoaded(t, start, options...), model, edited)
		})
	}

	t.Run("a --set on text takes the value as it is written", func(t *testing.T) {
		dir := t.TempDir()
		withLog := noOutputYAML + "outputs:\n- {name: log, function: {type: json_log, path: " + dir + "/a.log}}\n"
		config, err := LoadConfig(writeConfigPath(t, withLog),
			WithSet("outputs[name=log].function.path", dir+"/set #1: x.log"))
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunWith(config, WithConfigOutputs(), CaptureView("all", nil))
		if err != nil {
			t.Fatal(err)
		}
		assertSameEntries(t, "set path", keyedEntries(t, filepath.Join(dir, "set #1: x.log")),
			storageEntries(result.Views["all"]))
	})

	t.Run("a --set on the CLI matches the value edited into the file", func(t *testing.T) {
		var err error
		printed := captureStdout(t, func() {
			err = Execute([]string{"stochadex", "--config", writeConfigPath(t, noOutputYAML),
				"--set", firstSeed + "=99", "--set", maxSteps + "=12"})
		})
		if err != nil {
			t.Fatal(err)
		}
		edited := replaceOnce(t, replaceOnce(t, noOutputYAML, "seed: 7167", "seed: 99"),
			"max_steps: 40", "max_steps: 12")
		reference := runLoaded(t, edited)
		assertSameLines(t, "--set stdout", sortedLines(printed), rowLines(reference, ""))
	})

	t.Run("every ensemble member runs the overridden config", func(t *testing.T) {
		// Members are rebuilt from the loaded document, so a member re-reading
		// the file would step its partition with variances [1.0], not [4.0].
		// (max_steps alone could not show it: members share the loaded
		// simulation block.)
		ensemble := model + "run: {mode: ensemble, seeds: [5, 6]}\n"
		config, err := LoadConfig(writeConfigPath(t, ensemble),
			WithSet(maxSteps, "12"), WithSet(secondParam, "[4.0]"), WithSet("run.seeds", "[8, 9, 10]"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		want, err := RunToStorage(writeConfig(t, strings.NewReplacer("max_steps: 40", "max_steps: 12",
			"variances: [1.0]\n", "variances: [4.0]\n", "seeds: [5, 6]", "seeds: [8, 9, 10]").Replace(ensemble)))
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Members) != 3 || len(want.Members) != 3 {
			t.Fatalf("got %d members, want 3 on both sides", len(got.Members))
		}
		for i := range want.Members {
			if got.Members[i].Seed != want.Members[i].Seed {
				t.Fatalf("member %d has seed %d, want %d", i, got.Members[i].Seed, want.Members[i].Seed)
			}
			if rows := len(got.Members[i].Storage.GetTimes()); rows != 13 {
				t.Fatalf("member %d ran %d rows, want 13", i, rows)
			}
			assertStoragesEqual(t, fmt.Sprintf("member %d", i), got.Members[i].Storage, want.Members[i].Storage)
		}
	})

	t.Run("every served connection runs the overridden config", func(t *testing.T) {
		served := servedYAML(model, "127.0.0.1:0")
		config, err := LoadConfig(writeConfigPath(t, served), WithSet(maxSteps, "12"), WithSet(firstSeed, "99"))
		if err != nil {
			t.Fatal(err)
		}
		handler, err := serveHandler(config)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		want := capturedRun(t, replaceOnce(t, replaceOnce(t, model, "max_steps: 40", "max_steps: 12"),
			"seed: 7167", "seed: 99"), nil)
		for connection := 0; connection < 2; connection++ {
			assertMatchesReference(t, fmt.Sprintf("connection %d", connection),
				readStream(t, wsURLOf(server)), want)
		}
	})

	t.Run("a --set replaces a placeholder, whose variable then need not be set", func(t *testing.T) {
		templated := replaceOnce(t, model, "seed: 7167", "seed: ${UNSET_SEED}")
		got := runLoaded(t, templated, WithSet(firstSeed, "99"), envOf(nil))
		assertEditedRun(t, got, model, replaceOnce(t, model, "seed: 7167", "seed: 99"))
	})

	t.Run("a config with no overrides is decoded from the file's own bytes", func(t *testing.T) {
		data := []byte(model)
		resolved, err := resolveSource(data, nil)
		if err != nil || resolved.blame != nil {
			t.Fatal(err)
		}
		if &resolved.source[0] != &data[0] {
			t.Error("a config with nothing to resolve should not be re-encoded")
		}
	})
}

func TestSetOverrideErrors(t *testing.T) {
	model := serveConfigYAML
	aliased := replaceOnce(t, replaceOnce(t, model, "    params:\n      variances: [1.0, 1.0]",
		"    params: &shared\n      variances: [1.0, 1.0]"), "    params:\n      variances: [1.0]", "    params: *shared")
	secondBlock := model[strings.Index(model, "  - name: second"):strings.Index(model, "  simulation:")]
	duplicated := replaceOnce(t, model, "  simulation:", secondBlock+"  simulation:")
	cases := []struct{ name, config, path, value, want string }{
		{"an unknown key", model, "main.partitons.seed", "1",
			"--set main.partitons.seed=1: the config has no main.partitons"},
		{"an unknown entry name", model, "main.partitions[name=nope].seed", "1",
			`main.partitions has no entry named "nope"`},
		{"a name selector on a mapping", model, "main.simulation[name=x].seed", "1",
			"main.simulation is not a list, so [name=x] selects nothing"},
		{"a positional index", model, "main.partitions[0].seed", "1",
			`"partitions[0]" is not a path segment`},
		{"a key under a scalar", model, firstSeed + ".x", "1",
			"main.partitions[name=first_wiener_process].seed is not a mapping, so it has no x"},
		{"text for a number", model, maxSteps, "many",
			"--set " + maxSteps + "=many: the config has a number here, not text"},
		{"a number for a list", model, secondParam, "4.0",
			"the config has a list here, not a number"},
		{"a number for a mapping", model, "main.simulation.timestep_function", "0.5",
			"the config has a mapping here, not a number"},
		{"a float for an int field", model, maxSteps, "1.5",
			"--set " + maxSteps + `=1.5: component "number_of_steps": field "max_steps" must be an integer, got float64`},
		{"a replacement mapping with a dead key", model, "main.simulation.termination_condition",
			"{type: number_of_steps, max_stpes: 5}",
			"--set main.simulation.termination_condition={type: number_of_steps, max_stpes: 5}: " +
				`component "number_of_steps": missing required field "max_steps"`},
		{"a negative seed, which the decoder rejects", model, firstSeed, "-1",
			"--set " + firstSeed + "=-1: yaml: unmarshal errors:\n  cannot unmarshal !!int `-1` into uint64"},
		{"a name selector on a list of lists", "x: [[name, q]]\n", "x[name=q]", "1",
			`x has no entry named "q"`},
		{"a value that is not YAML", model, secondParam, "[4.0",
			"the value is not valid YAML"},
		{"two entries with the name", duplicated, "main.partitions[name=second_wiener_process].seed", "1",
			`main.partitions has more than one entry named "second_wiener_process"`},
		{"a path through an alias", aliased, secondParam, "[2.0]",
			"main.partitions[name=second_wiener_process].params is a YAML alias"},
		{"an empty path segment", model, "main..seed", "1", `"" is not a path segment`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error naming the override", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.config), WithSet(c.path, c.value))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("an error in the file itself is reported as the file's, at the file's line", func(t *testing.T) {
		// Re-encoding drops the blank lines, so only the file's own error says line 24.
		broken := replaceOnce(t, replaceOnce(t, model, "init_time_value: 0.0", "init_time_value: zero"),
			"  simulation:", "\n\n  simulation:")
		_, err := LoadConfig(writeConfigPath(t, broken), WithSet(firstSeed, "99"), WithSet(maxSteps, "12"))
		if KindOf(err) != ErrConfig || strings.Contains(err.Error(), "--set") ||
			!strings.Contains(err.Error(), "line 24: cannot unmarshal !!str `zero` into float64") {
			t.Errorf("expected the file's own error, got %v", err)
		}
	})

	t.Run("the first --set at fault is named, not a later one", func(t *testing.T) {
		_, err := LoadConfig(writeConfigPath(t, model),
			WithSet(firstSeed, "99"), WithSet(maxSteps, "1.5"), WithSet(maxSteps, "2.5"))
		if want := "--set " + maxSteps + "=1.5: "; KindOf(err) != ErrConfig || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("expected an error starting %q, got %v", want, err)
		}
	})

	for _, arg := range []string{"no-equals-sign", "=7", "main.partitions[name=w=1].seed"} {
		t.Run(fmt.Sprintf("--set %s is a usage error", arg), func(t *testing.T) {
			err := Execute([]string{"stochadex", "--config", writeConfigPath(t, model), "--set", arg})
			if KindOf(err) != ErrUsage || !strings.Contains(err.Error(), "expected path=value") {
				t.Errorf("expected ErrUsage, got %v", err)
			}
		})
	}
}

func TestPlaceholders(t *testing.T) {
	model := serveConfigYAML
	t.Run("placeholders anywhere in a value match the values written in", func(t *testing.T) {
		templated := strings.NewReplacer(
			"seed: 7167", "seed: ${SEED}",
			"variances: [1.0, 1.0]", "variances: [${V}, 1.0]",
			"variances: [1.0]\n", "variances: ${VS}\n",
			"{type: number_of_steps, max_steps: 40}", "{type: ${TERM}, max_steps: ${STEPS}}",
		).Replace(model)
		edited := strings.NewReplacer(
			"seed: 7167", "seed: 99",
			"variances: [1.0, 1.0]", "variances: [2.5, 1.0]",
			"variances: [1.0]\n", "variances: [4.0]\n",
			"max_steps: 40", "max_steps: 12",
		).Replace(model)
		got := runLoaded(t, templated, envOf(map[string]string{
			"SEED": "99", "V": "2.5", "VS": "[4.0]", "TERM": "number_of_steps", "STEPS": "12"}))
		assertEditedRun(t, got, model, edited)
	})

	t.Run("placeholders read the process environment by default, on the CLI", func(t *testing.T) {
		t.Setenv("STOCHADEX_TEST_SEED", "99")
		templated := replaceOnce(t, noOutputYAML, "seed: 7167", "seed: ${STOCHADEX_TEST_SEED}")
		var err error
		printed := captureStdout(t, func() {
			err = Execute([]string{"stochadex", "--config", writeConfigPath(t, templated)})
		})
		if err != nil {
			t.Fatal(err)
		}
		reference := runLoaded(t, replaceOnce(t, noOutputYAML, "seed: 7167", "seed: 99"))
		assertSameLines(t, "placeholder stdout", sortedLines(printed), rowLines(reference, ""))
	})

	t.Run("a placeholder inside text fills in text and an escaped one is kept", func(t *testing.T) {
		dir := t.TempDir()
		templated := noOutputYAML + "outputs:\n" +
			"- {name: plain, function: {type: json_log, path: " + dir + "/plain-${RUN}.log}}\n" +
			"- {name: quoted, function: {type: json_log, path: \"" + dir + "/quoted #${RUN}.log\"}}\n" +
			"- {name: escaped, function: {type: json_log, path: \"" + dir + "/$${RUN}.log\"}}\n"
		config, err := LoadConfig(writeConfigPath(t, templated), envOf(map[string]string{"RUN": "7"}))
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunWith(config, WithConfigOutputs(), CaptureView("all", nil))
		if err != nil {
			t.Fatal(err)
		}
		want := storageEntries(result.Views["all"])
		for _, name := range []string{"plain-7.log", "quoted #7.log", "${RUN}.log"} {
			assertSameEntries(t, name, keyedEntries(t, filepath.Join(dir, name)), want)
		}
	})

	t.Run("a quoted placeholder stays text where an unquoted one reads as YAML", func(t *testing.T) {
		templated := replaceOnce(t, model, "variances: [1.0]\n", "variances: \"${VS}\"\n")
		_, err := LoadConfig(writeConfigPath(t, templated), envOf(map[string]string{"VS": "[4.0]"}))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "(with ${VS}=[4.0])") ||
			!strings.Contains(err.Error(), "cannot unmarshal !!str `[4.0]`") {
			t.Errorf("expected the quoted list to be text, got %v", err)
		}
	})

	t.Run("a placeholder in a comment is ignored", func(t *testing.T) {
		got := runLoaded(t, "# seed with ${NEVER_SET} to vary the run\n"+model, envOf(nil))
		assertStoragesEqual(t, "commented", got, runLoaded(t, model))
	})

	templated := replaceOnce(t, model, "seed: 7167", "seed: ${SEED}")
	cases := []struct {
		name, config string
		env          map[string]string
		want         string
	}{
		{"an unset variable", templated, nil, "line 9: ${SEED} is not set"},
		{"an empty variable", templated, map[string]string{"SEED": ""},
			"line 9: ${SEED} is set but empty"},
		{"a placeholder in a key", replaceOnce(t, model, "variances: [1.0]", "${KEY}: [1.0]"),
			map[string]string{"KEY": "variances"},
			"line 13: placeholders are only allowed in values, not in the key ${KEY}"},
		{"a malformed name", replaceOnce(t, model, "seed: 7167", "seed: ${1X}"), nil,
			"line 9: ${1X} is not a placeholder"},
		{"an unclosed placeholder", replaceOnce(t, model, "seed: 7167", "seed: ${SEED"), nil,
			"line 9: unclosed ${ placeholder"},
		{"a value that is not YAML", replaceOnce(t, model, "variances: [1.0]\n", "variances: ${VS}\n"),
			map[string]string{"VS": "[4.0"}, "line 13: ${VS}, filled in, is not valid YAML"},
		{"a value of the wrong type", replaceOnce(t, model, "max_steps: 40", "max_steps: ${STEPS}"),
			map[string]string{"STEPS": "many"},
			`component "number_of_steps": field "max_steps" must be an integer, got string (with ${STEPS}=many)`},
	}
	for _, c := range cases {
		t.Run(c.name+" is a config error naming it", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.config), envOf(c.env))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("an empty config with an override is a config error", func(t *testing.T) {
		_, err := LoadConfig(writeConfigPath(t, "# nothing\n"), WithSet("run.seeds", "[1]"))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "the config is empty") {
			t.Errorf("expected an empty-config error, got %v", err)
		}
	})
}

// reencoded is a document after a pass through the override machinery with
// nothing applied.
func reencoded(t *testing.T, data []byte) []byte {
	t.Helper()
	p, err := tokenize(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := render(p, loadOptions{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertDecodesAlike(t *testing.T, label string, data []byte) {
	t.Helper()
	var want, got ApiRunConfig
	if err := yaml.Unmarshal(data, &want); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	out := reencoded(t, data)
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("%s re-encoded: %v\n%s", label, err, out)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s decodes differently once re-encoded:\n%s", label, out)
	}
}

func TestOverridesKeepEveryConfigIntact(t *testing.T) {
	t.Run("every shipped config decodes the same once re-encoded", func(t *testing.T) {
		paths := []string{}
		for _, pattern := range []string{"../../cfg/example_*.yaml", "../../models/*/declarative.yaml",
			"../../.claude/skills/stochadex-model/recipes/*.yaml"} {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				t.Fatal(err)
			}
			paths = append(paths, matches...)
		}
		if len(paths) < 30 {
			t.Fatalf("found only %d configs", len(paths))
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			assertDecodesAlike(t, path, data)
		}
	})

	t.Run("YAML 1.1 spellings yaml.v2 reads specially survive re-encoding", func(t *testing.T) {
		// A generic yaml.v2 round trip turns the key n into false and y into true.
		hazards := `main:
  partitions:
  - name: hazard
    iteration: {type: wiener_process}
    params: {n: [1.0], y: [2.0], "yes": [3.0], on: [4.0], off: [5.0], no: [6.0]}
    init_state_values: [1.0e-3, 0x10, .5, 1_000]
    seed: 18446744073709551615
    state_history_depth: 1
  simulation: &sim
    init_time_value: 0.0
data:
  steps: 3
  timestep: 1.0
  partitions: []
macros:
- type: scalar_regression_stats
  name: r
  intercept: yes
  y: {partition_name: a}
  x: {partition_name: b}
- type: scalar_regression_stats
  name: s
  intercept: off
  y: {partition_name: a}
  x: {partition_name: b}
`
		assertDecodesAlike(t, "hazards", []byte(hazards))
		var decoded ApiRunConfig
		if err := yaml.Unmarshal(reencoded(t, []byte(hazards)), &decoded); err != nil {
			t.Fatal(err)
		}
		params := decoded.Main.Partitions[0].Params.Map
		for _, key := range []string{"n", "y", "yes", "on", "off", "no"} {
			if _, ok := params[key]; !ok {
				t.Errorf("the param %q did not survive: %v", key, params)
			}
		}
	})

	t.Run("a --set on a YAML 1.1 boolean spelling takes a boolean", func(t *testing.T) {
		config := "macros:\n- {type: scalar_regression_stats, name: r, intercept: yes, " +
			"y: {partition_name: a}, x: {partition_name: b}}\n"
		intercept := func(value string) (bool, error) {
			resolved, err := resolveSource([]byte(config), []LoadOption{WithSet("macros[name=r].intercept", value)})
			if err != nil {
				return false, err
			}
			var decoded ApiRunConfig
			if err := yaml.Unmarshal(resolved.source, &decoded); err != nil {
				return false, err
			}
			return decoded.Macros[0].Spec.(*scalarRegressionStatsSpec).Intercept, nil
		}
		for value, want := range map[string]bool{"false": false, "no": false, "on": true} {
			if got, err := intercept(value); err != nil || got != want {
				t.Errorf("intercept=%s gave %v (%v), want %v", value, got, err, want)
			}
		}
		if _, err := intercept("5"); err == nil || !strings.Contains(err.Error(), "a boolean here, not a number") {
			t.Errorf("expected a shape error, got %v", err)
		}
	})

	t.Run("a pass with nothing to apply leaves every line where it was", func(t *testing.T) {
		p, err := tokenize([]byte("a: ${X}\nb: [${Y}, $${Z}]\n# ${W}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if lines := bytes.Count(p.tokenized, []byte("\n")); lines != 3 {
			t.Errorf("tokenized document has %d lines, want 3", lines)
		}
		if strings.Join(p.variables, ",") != "X,Y,W" {
			t.Errorf("variables %v, want X,Y,W in order of appearance", p.variables)
		}
	})
}
