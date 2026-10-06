package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gopkg.in/yaml.v2"
)

// assertSameStorage fails unless two storages hold the same times and, for
// every partition of want, the same rows.
func assertSameStorage(t *testing.T, label string, got, want *simulator.StateTimeStorage) {
	t.Helper()
	if !floats.Equal(got.GetTimes(), want.GetTimes()) {
		t.Fatalf("%s: times differ (%d vs %d rows)", label, len(got.GetTimes()), len(want.GetTimes()))
	}
	for _, name := range want.GetNames() {
		wantRows, gotRows := want.GetValues(name), got.GetValues(name)
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

// TestSolarFleetDrivenFromAnInput is PLAN.md item 1.2's acceptance test: the
// solar-fleet catalogue model drives its sites from a clear-sky series written
// inline with {type: from_storage}. Moving that series into a CSV input and
// replaying it with {type: from_input} — with the clock taken from the input's
// times and the run stopping when the input is exhausted — must reproduce the
// model exactly.
func TestSolarFleetDrivenFromAnInput(t *testing.T) {
	const declarative = "../../models/solar-fleet/declarative.yaml"
	reference, err := RunToStorage(LoadApiRunConfigFromYaml(declarative))
	if err != nil {
		t.Fatal(err)
	}

	var document map[interface{}]interface{}
	if err := yaml.Unmarshal([]byte(readFile(t, declarative)), &document); err != nil {
		t.Fatal(err)
	}
	main := document["main"].(map[interface{}]interface{})
	var rows []interface{}
	for _, entry := range main["partitions"].([]interface{}) {
		partition := entry.(map[interface{}]interface{})
		if partition["name"] != "clearsky" {
			continue
		}
		iteration := partition["iteration"].(map[interface{}]interface{})
		rows = iteration["data"].([]interface{})
		partition["iteration"] = map[interface{}]interface{}{"type": "from_input", "input": "sky"}
	}
	if len(rows) == 0 {
		t.Fatal("solar-fleet's clearsky from_storage rows were not found")
	}

	// The CSV holds the same rows, one per step: time, then the 4 site values,
	// formatted so they parse back to exactly the same float64s.
	var csv strings.Builder
	for step, row := range rows {
		fields := []string{strconv.Itoa(step)}
		for _, value := range row.([]interface{}) {
			var number float64
			switch typed := value.(type) {
			case int:
				number = float64(typed)
			case float64:
				number = typed
			}
			fields = append(fields, strconv.FormatFloat(number, 'g', -1, 64))
		}
		csv.WriteString(strings.Join(fields, ",") + "\n")
	}
	csvPath := filepath.Join(t.TempDir(), "clearsky.csv")
	writeFile(t, csvPath, csv.String())

	simulation := main["simulation"].(map[interface{}]interface{})
	simulation["timestep_function"] = map[interface{}]interface{}{"type": "from_input", "input": "sky"}
	simulation["termination_condition"] = map[interface{}]interface{}{"type": "input_exhausted", "input": "sky"}
	document["inputs"] = map[interface{}]interface{}{"sky": map[interface{}]interface{}{
		"source": map[interface{}]interface{}{"csv": map[interface{}]interface{}{
			"path": csvPath, "time_column": 0,
			"state_columns": map[interface{}]interface{}{"clearsky": []interface{}{1, 2, 3, 4}},
		}},
	}}
	rewritten, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rewritten), "from_storage") {
		t.Fatal("the rewritten config still replays clearsky inline")
	}

	fromInput, err := RunToStorage(writeConfig(t, string(rewritten)))
	if err != nil {
		t.Fatal(err)
	}
	assertSameStorage(t, "solar-fleet driven from a CSV input", fromInput.Storage, reference.Storage)
	if n := len(reference.Storage.GetTimes()); n != len(rows) {
		t.Errorf("expected the run to cover all %d input rows, got %d", len(rows), n)
	}
}

// inputsMain is a run whose partition "replay" replays an input, plus a seeded
// walk so the run has dynamics of its own. INPUTS and REPLAY are filled per test.
const inputsMain = `inputs:
INPUTS
main:
  partitions:
  - name: replay
    iteration: REPLAY
    state_history_depth: 1
    seed: 0
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 3
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: input_exhausted, input: src}
    timestep_function: {type: from_input, input: src}
    init_time_value: 0.0
`

func inputsConfig(inputs, replay string) string {
	return strings.NewReplacer("INPUTS", inputs, "REPLAY", replay).Replace(inputsMain)
}

// simulationInput is a 2-wide random walk named "source", run for 12 steps of
// 0.5 as a pre-pass input.
const simulationInput = `  src:
    simulation:
      steps: 12
      timestep: 0.5
      partitions:
      - {name: source, iteration: {type: wiener_process}, params: {variances: [1.0, 2.0]}, init_state_values: [1.0, -1.0], state_history_depth: 1, seed: 9}`

func TestInputs(t *testing.T) {
	t.Run("a simulation input replays exactly, with its clock and length", func(t *testing.T) {
		// The input's own rows, computed independently by running it as a data: block.
		reference, err := (&DataConfig{
			Steps: 12, Timestep: 0.5,
			Partitions: []simulator.PartitionConfig{{
				Name: "source", IterationSpec: simulator.ComponentSpec{Type: "wiener_process"},
				Params:          simulator.NewParams(map[string][]float64{"variances": {1.0, 2.0}}),
				InitStateValues: []float64{1.0, -1.0}, StateHistoryDepth: 1, Seed: 9,
			}},
		}).buildStorage()
		if err != nil {
			t.Fatal(err)
		}
		// init_state_values omitted: it defaults to the input's first row.
		result, err := RunToStorage(writeConfig(t, inputsConfig(simulationInput,
			"{type: from_input, input: src, partition: source}")))
		if err != nil {
			t.Fatal(err)
		}
		want := reference.GetValues("source")
		got := result.Storage.GetValues("replay")
		if len(got) != len(want) {
			t.Fatalf("replay has %d rows, want the input's %d", len(got), len(want))
		}
		for i := range want {
			if !floats.Equal(got[i], want[i]) {
				t.Fatalf("replay row %d = %v, want %v", i, got[i], want[i])
			}
		}
		if !floats.Equal(result.Storage.GetTimes(), reference.GetTimes()) {
			t.Errorf("the run's clock %v should follow the input's times %v",
				result.Storage.GetTimes(), reference.GetTimes())
		}
	})

	t.Run("a json_log input replays exactly", func(t *testing.T) {
		// A run's own json_log output becomes the next run's input — the
		// file-chained pipeline the plan relies on.
		logPath := filepath.Join(t.TempDir(), "previous.log")
		previous := writeConfig(t, fmt.Sprintf(batchConfigYAML, "{type: every_step}", logPath))
		reference, err := RunToStorage(previous)
		if err != nil {
			t.Fatal(err)
		}
		Run(previous, &SocketConfig{})
		result, err := RunToStorage(writeConfig(t, inputsConfig(
			fmt.Sprintf("  src:\n    source: {json_log: {path: %q}}", logPath),
			"{type: from_input, input: src, partition: first}")))
		if err != nil {
			t.Fatal(err)
		}
		want, got := reference.Storage.GetValues("first"), result.Storage.GetValues("replay")
		if len(got) != len(want) {
			t.Fatalf("replay has %d rows, want %d", len(got), len(want))
		}
		for i := range want {
			if !floats.Equal(got[i], want[i]) {
				t.Fatalf("replay row %d = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("inputs are read when a run starts, not at load", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "later.csv")
		config, err := LoadConfig(writeConfigPath(t, inputsConfig(
			fmt.Sprintf("  src:\n    source: {csv: {path: %q, time_column: 0, state_columns: {d: [1]}}}", path),
			"{type: from_input, input: src, partition: d}")))
		if err != nil {
			t.Fatalf("loading should not read the input, but failed: %v", err)
		}
		// The input appears after the config was loaded; the run still reads it.
		writeFile(t, path, "0,1.5\n1,2.5\n2,3.5\n")
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Storage.GetValues("replay"); len(got) != 3 || got[2][0] != 3.5 {
			t.Errorf("replay = %v, want the input's rows 1.5, 2.5, 3.5", got)
		}
	})

	t.Run("each ensemble member replays the input", func(t *testing.T) {
		result, err := RunToStorage(writeConfig(t, inputsConfig(simulationInput,
			"{type: from_input, input: src, partition: source}")+"run: {mode: ensemble, seeds: [1, 2]}\n"))
		if err != nil {
			t.Fatal(err)
		}
		first, second := result.Members[0].Storage, result.Members[1].Storage
		assertSameStorage(t, "replayed input across members",
			storageWith(first, "replay"), storageWith(second, "replay"))
		if floats.Equal(first.GetValues("walk")[12], second.GetValues("walk")[12]) {
			t.Error("the members' own walks should differ by seed")
		}
	})

	failures := []struct {
		name   string
		config func(t *testing.T) string
		kind   ErrorKind
		want   string
	}{
		{"a missing input file", func(t *testing.T) string {
			return inputsConfig(fmt.Sprintf("  src:\n    source: {csv: {path: %q, time_column: 0, state_columns: {d: [1]}}}",
				filepath.Join(t.TempDir(), "absent.csv")), "{type: from_input, input: src, partition: d}")
		}, ErrUnavailable, "absent.csv"},
		{"an input without the named partition", func(t *testing.T) string {
			return inputsConfig(simulationInput, "{type: from_input, input: src, partition: ghost}")
		}, ErrData, `has no partition "ghost" (it has: source)`},
		{"init_state_values of the wrong width", func(t *testing.T) string {
			return strings.Replace(inputsConfig(simulationInput, "{type: from_input, input: src, partition: source}"),
				"    state_history_depth: 1\n    seed: 0\n", "    init_state_values: [0.0]\n    state_history_depth: 1\n    seed: 0\n", 1)
		}, ErrData, "1 init_state_values"},
	}
	for _, c := range failures {
		t.Run(c.name+" fails the run, classified", func(t *testing.T) {
			_, err := RunToStorage(writeConfig(t, c.config(t)))
			if KindOf(err) != c.kind || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected kind %d mentioning %q, got %v", c.kind, c.want, err)
			}
		})
	}

	loadErrors := []struct{ name, yaml, want string }{
		{"an undeclared input", inputsConfig(simulationInput, "{type: from_input, input: nope, partition: source}"),
			`names input "nope"`},
		{"an unused input", inputsConfig(simulationInput+"\n  spare:\n    source: {json_log: {path: x.log}}",
			"{type: from_input, input: src, partition: source}"), `input "spare" is never used`},
		{"an input with both source and simulation", inputsConfig(simulationInput+"\n    source: {json_log: {path: x.log}}",
			"{type: from_input, input: src, partition: source}"), "exactly one of"},
		{"from_input without an input", inputsConfig(simulationInput, "{type: from_input, partition: source}"),
			"needs input:"},
		{"a clock naming an undeclared input", strings.Replace(inputsConfig(simulationInput,
			"{type: from_input, input: src, partition: source}"), "{type: from_input, input: src}", "{type: from_input, input: nope}", 1),
			`names input "nope"`},
		{"inputs: with macros:", macroConfigYAML + "inputs:\n" + simulationInput + "\n", "not yet available to macros:"},
	}
	for _, c := range loadErrors {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig mentioning %q, got %v", c.want, err)
			}
		})
	}
}

// writeConfigPath writes a config file and returns its path, without loading it.
func writeConfigPath(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// storageWith returns a storage holding only one partition of another, with its
// times, so two storages can be compared on that partition alone.
func storageWith(source *simulator.StateTimeStorage, name string) *simulator.StateTimeStorage {
	storage := simulator.NewStateTimeStorage()
	storage.SetValues(name, source.GetValues(name))
	storage.SetTimes(source.GetTimes())
	return storage
}

func TestInputSpecsAndGuards(t *testing.T) {
	replay := "{type: from_input, input: src, partition: source}"
	t.Run("a config run without binding its inputs fails loudly, pointing at the API", func(t *testing.T) {
		config := writeConfig(t, inputsConfig(simulationInput, replay))
		message := func() (message string) {
			defer func() { message = stringify(recover()) }()
			simulator.NewPartitionCoordinator(config.GetConfigGenerator().GenerateConfigs())
			return ""
		}()
		if !strings.Contains(message, "unbound") || !strings.Contains(message, "RunWith") {
			t.Errorf("expected a panic explaining the partition is unbound and how to run it, got %q", message)
		}
		for name, call := range map[string]func(){
			"from_input iteration": func() { (&unboundInputIteration{input: "src"}).Iterate(nil, 0, nil, nil) },
			"from_input timestep":  func() { (&unboundInputTimesteps{input: "src"}).NextIncrement(nil) },
			"input_exhausted":      func() { (&unboundInputExhausted{input: "src"}).Terminate(nil, nil) },
		} {
			if !didPanic(call) {
				t.Errorf("an unbound %s should panic rather than run", name)
			}
		}
	})

	t.Run("an input used only by the clock is still read, and classified", func(t *testing.T) {
		yaml := strings.Replace(inputsConfig(simulationInput+
			"\n  clock:\n    source: {json_log: {path: "+fmt.Sprintf("%q", filepath.Join(t.TempDir(), "absent.log"))+"}}",
			replay), "{type: from_input, input: src}", "{type: from_input, input: clock}", 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), `input "clock"`) {
			t.Errorf("expected ErrUnavailable naming the clock's input, got %v", err)
		}
	})

	loadErrors := []struct{ name, yaml, want string }{
		{"a misspelled from_input field", inputsConfig(simulationInput, "{type: from_input, input: src, partiton: source}"),
			`unknown field "partiton"`},
		{"a non-string input name", inputsConfig(simulationInput, "{type: from_input, input: 3}"),
			"input must be a string"},
		{"a clock without an input", strings.Replace(inputsConfig(simulationInput, replay),
			"{type: from_input, input: src}", "{type: from_input}", 1), "needs input:"},
		{"input_exhausted with a misspelled field", strings.Replace(inputsConfig(simulationInput, replay),
			"{type: input_exhausted, input: src}", "{type: input_exhausted, inptu: src}", 1), `unknown field "inptu"`},
		{"input_exhausted naming an undeclared input", strings.Replace(inputsConfig(simulationInput, replay),
			"{type: input_exhausted, input: src}", "{type: input_exhausted, input: nope}", 1), `names input "nope"`},
		{"an input with neither source nor simulation", inputsConfig("  src: {}", replay), "exactly one of"},
		{"a simulation input that also sets a source", inputsConfig(simulationInput+
			"\n      source: {json_log: {path: x.log}}", replay), "cannot also set source:"},
		{"from_input inside an embedded run", `inputs:
` + simulationInput + `
main:
  partitions:
  - {name: inner_run, params: {burn_in_steps: [0]}, init_state_values: [0.0, 0.0], state_history_depth: 1, seed: 0}
  - {name: replay, iteration: {type: from_input, input: src, partition: source}, state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
embedded:
- name: inner_run
  partitions:
  - {name: p, iteration: {type: from_input, input: src, partition: source}, state_history_depth: 1, seed: 0}
  simulation:
    output_condition: {type: nil}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, "not yet supported inside embedded runs"},
	}
	for _, c := range loadErrors {
		t.Run(c.name+" is a config error at load", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig mentioning %q, got %v", c.want, err)
			}
		})
	}
}
