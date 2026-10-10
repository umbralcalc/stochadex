package api

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// fileSizeIteration reads the file its model_path names when it is configured,
// and outputs the file's size: a stand-in for a model partition that proves
// the path it was given reaches a real file.
type fileSizeIteration struct {
	path string
	size float64
}

func (f *fileSizeIteration) Configure(int, *simulator.Settings) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		panic(err)
	}
	f.size = float64(len(data))
}

func (f *fileSizeIteration) Iterate(
	*simulator.Params, int, []*simulator.StateHistory, *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{f.size}
}

func init() {
	RegisterIteration("file_size", func(spec simulator.ComponentSpec) (simulator.Iteration, error) {
		path, ok := spec.Fields["model_path"].(string)
		if !ok {
			return nil, fmt.Errorf("file_size: model_path must be a path, got %T", spec.Fields["model_path"])
		}
		return &fileSizeIteration{path: path}, nil
	})
}

// modelFiles writes two model files of known sizes, returning their paths.
func modelFiles(t *testing.T) (string, string) {
	dir := t.TempDir()
	brain, other := filepath.Join(dir, "brain.onnx"), filepath.Join(dir, "other.onnx")
	writeFileAt(t, brain, strings.Repeat("w", 123))
	writeFileAt(t, other, strings.Repeat("v", 45))
	return brain, other
}

func sizedPartition(name, model string) string {
	return "  - {name: " + name + ", iteration: {type: file_size, model_path: " + model +
		"}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n"
}

func lastValue(t *testing.T, yaml, partition string) float64 {
	t.Helper()
	result, err := RunToStorage(writeConfig(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	rows := result.Storage.GetValues(partition)
	return rows[len(rows)-1][0]
}

func TestFileInputs(t *testing.T) {
	brain, other := modelFiles(t)
	declared := fmt.Sprintf("inputs:\n  brain: {file: {path: %q}}\n", brain)

	t.Run("an iteration reads a declared file through {input: NAME}", func(t *testing.T) {
		yaml := declared + checkYAML(sizedPartition("policy", "{input: brain}"), "")
		if got := lastValue(t, yaml, "policy"); got != 123 {
			t.Errorf("the partition read %v bytes, want the file's 123", got)
		}
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		manifest := Manifest(config)
		if len(manifest.Inputs) != 1 || manifest.Inputs[0].Kind != "file" ||
			manifest.Inputs[0].Location != brain ||
			!slices.Equal(manifest.Inputs[0].ReadBy, []string{"main.partitions[name=policy].iteration.model_path"}) {
			t.Errorf("manifest inputs %+v", manifest.Inputs)
		}
	})

	t.Run("a bare model_path is an implicit file input, shared by partitions naming one file", func(t *testing.T) {
		shorthand := checkYAML(sizedPartition("a", fmt.Sprintf("%q", brain))+
			sizedPartition("b", fmt.Sprintf("%q", brain))+sizedPartition("c", fmt.Sprintf("%q", other)), "")
		explicit := fmt.Sprintf("inputs:\n  m1: {file: {path: %q}}\n  m2: {file: {path: %q}}\n", brain, other) +
			checkYAML(sizedPartition("a", "{input: m1}")+sizedPartition("b", "{input: m1}")+sizedPartition("c", "{input: m2}"), "")
		got, err := RunToStorage(writeConfig(t, shorthand))
		if err != nil {
			t.Fatal(err)
		}
		want, err := RunToStorage(writeConfig(t, explicit))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "shorthand vs declared", got.Storage, want.Storage)
		config, err := LoadConfig(writeConfigPath(t, shorthand))
		if err != nil {
			t.Fatal(err)
		}
		inputs := Manifest(config).Inputs
		if len(inputs) != 2 || inputs[0].Name != implicitFilePrefix+brain ||
			!slices.Equal(inputs[0].ReadBy, []string{"main.partitions[name=a].iteration.model_path",
				"main.partitions[name=b].iteration.model_path"}) || inputs[1].Location != other {
			t.Errorf("implicit inputs %+v", inputs)
		}
	})

	t.Run("an embedded run's iteration reads a declared file", func(t *testing.T) {
		yaml := declared + checkYAML(hostPartition, `embedded:
- name: nested
  partitions:
`+sizedPartition("inner", "{input: brain}")+`  simulation:
    termination_condition: {type: number_of_steps, max_steps: 2}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`)
		if got := lastValue(t, yaml, "nested"); got != 123 {
			t.Errorf("the host row holds %v, want the inner partition's 123", got)
		}
		config, _ := LoadConfig(writeConfigPath(t, yaml))
		if readBy := Manifest(config).Inputs[0].ReadBy; !slices.Equal(readBy,
			[]string{"embedded[name=nested].partitions[name=inner].iteration.model_path"}) {
			t.Errorf("read_by %v", readBy)
		}
	})

	t.Run("a reference nested in a sub-spec is resolved too", func(t *testing.T) {
		yaml := declared + checkYAML("  - {name: r, iteration: {type: model_reader, model_path: x.onnx, "+
			"options: {weights: {input: brain}}, stages: [{input: brain}], scaled: {input: brain, scale: 2}}, "+
			"init_state_values: [0.0], state_history_depth: 1, seed: 0}\n", "")
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		fields := config.Main.Partitions[0].IterationSpec.Fields
		weights := fields["options"].(map[interface{}]interface{})["weights"]
		stage := fields["stages"].([]interface{})[0]
		if weights != brain || stage != brain {
			t.Errorf("nested references resolved to %v and %v, want %s", weights, stage, brain)
		}
		// Only a mapping with the one key input: is a reference; a sub-spec that
		// has an input: field among others is left as written.
		if scaled, ok := fields["scaled"].(map[interface{}]interface{}); !ok || scaled["input"] != "brain" {
			t.Errorf("a sub-spec with an input field was taken for a reference: %v", fields["scaled"])
		}
		var readers []string
		for _, input := range Manifest(config).Inputs {
			if input.Name == "brain" {
				readers = input.ReadBy
			}
		}
		if !slices.Equal(readers, []string{"main.partitions[name=r].iteration.options.weights",
			"main.partitions[name=r].iteration.stages[0]"}) {
			t.Errorf("read_by %v", readers)
		}
	})

	t.Run("a macros config's sub-simulation reads a file, which its macros do not analyse", func(t *testing.T) {
		yaml := declared + `  data:
    simulation:
      steps: 20
      timestep: 1.0
      partitions:
` + strings.ReplaceAll(sizedPartition("stream", "{input: brain}"), "  - ", "      - ") + `macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [5.0]}
  window: 5
`
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		rows := result.Storage.GetValues("rolling_mean")
		if rows[len(rows)-1][0] != 123 {
			t.Errorf("the rolling mean of a constant 123 is %v", rows[len(rows)-1][0])
		}
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		kinds := map[string]string{}
		for _, input := range Manifest(config).Inputs {
			kinds[input.Name] = input.Kind
		}
		if kinds["brain"] != "file" || kinds["data"] != "simulation" {
			t.Errorf("a macros config's manifest should list its file input too: %v", kinds)
		}
	})

	t.Run("provenance fingerprints a file input by its contents", func(t *testing.T) {
		pinnedBuild(t, "pinned")
		yaml := declared + checkYAML(sizedPartition("policy", "{input: brain}"), "")
		key := func() *Provenance {
			config, err := LoadConfig(writeConfigPath(t, yaml))
			if err != nil {
				t.Fatal(err)
			}
			p, err := ComputeProvenance(config)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
		first := key()
		digest, _ := fileDigest(brain)
		if first.Key == "" || first.Inputs[0].Fingerprint != "sha256:"+digest {
			t.Fatalf("provenance %+v", first)
		}
		writeFileAt(t, brain, strings.Repeat("w", 124))
		defer writeFileAt(t, brain, strings.Repeat("w", 123))
		if key().Key == first.Key {
			t.Error("retraining the model left the key unchanged")
		}
	})

	t.Run("--check does not read a file input, in main or an embedded run", func(t *testing.T) {
		// The partitions read the file when configured, so a check that
		// configured them would fail on the missing file; the run does.
		missing := "inputs:\n  brain: {file: {path: /no/such/model.onnx}}\n"
		configs := map[string]string{
			"main": missing + checkYAML(sizedPartition("policy", "{input: brain}"), ""),
			"embedded": missing + checkYAML(hostPartition, "embedded:\n- name: nested\n  partitions:\n"+
				sizedPartition("inner", "{input: brain}")+"  simulation:\n    termination_condition: "+
				"{type: number_of_steps, max_steps: 2}\n    timestep_function: {type: constant, stepsize: 1.0}\n"+
				"    init_time_value: 0.0\n"),
		}
		for name, yaml := range configs {
			path := writeConfigPath(t, yaml)
			if err, _ := executeIn(t, t.TempDir(), "--config", path, "--check"); err != nil {
				t.Errorf("%s: --check read the model file: %v", name, err)
			}
			if err, _ := executeIn(t, t.TempDir(), "--config", path); err == nil {
				t.Errorf("%s: the run should fail without its model file", name)
			}
		}
	})

	t.Run("checking leaves an in-memory config's file readers to run", func(t *testing.T) {
		yaml := declared + checkYAML(hostPartition+sizedPartition("policy", "{input: brain}"),
			"embedded:\n- name: nested\n  partitions:\n"+sizedPartition("inner", "{input: brain}")+
				"  simulation:\n    termination_condition: {type: number_of_steps, max_steps: 2}\n"+
				"    timestep_function: {type: constant, stepsize: 1.0}\n    init_time_value: 0.0\n")
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		config.source = nil // as a config built in Go has no document
		if err := Check(config); err != nil {
			t.Fatal(err)
		}
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"policy", "nested"} {
			if rows := result.Storage.GetValues(name); rows[len(rows)-1][0] != 123 {
				t.Errorf("%s ran a stand-in after the check: %v", name, rows[len(rows)-1])
			}
		}
	})

	csvInput := "  obs: {source: {csv: {path: x.csv, time_column: 0, state_columns: {o: [1]}}}}\n"
	errorCases := []struct{ name, yaml, want string }{
		{"a reference to an undeclared input", checkYAML(sizedPartition("p", "{input: nope}"), ""),
			`main.partitions[name=p].iteration.model_path names input "nope", which inputs: does not declare`},
		{"a reference to an input that is not a file", "inputs:\n" + csvInput +
			checkYAML(sizedPartition("p", "{input: obs}"), ""),
			`names input "obs", which is not a file input`},
		{"a file input replayed by from_input", declared +
			checkYAML("  - {name: p, iteration: {type: from_input, input: brain}, state_history_depth: 1}\n", ""),
			`reads input "brain"'s rows, but it is a file`},
		{"a file input setting params", declared +
			checkYAML(walkPartition("w", ", params_from_input: {variances: {input: brain}}"), ""),
			`reads input "brain"'s rows, but it is a file`},
		{"an unused file input", declared + checkYAML(walkPartition("w", ""), ""),
			`input "brain" is never used`},
		{"a file input with no path", "inputs:\n  brain: {file: {}}\n" +
			checkYAML(sizedPartition("p", "{input: brain}"), ""), `input "brain": file: needs a path`},
		{"an input already named as a model file would be", fmt.Sprintf("inputs:\n  %q: {source: {csv: {path: x.csv, time_column: 0, state_columns: {o: [1]}}}}\n", implicitFilePrefix+brain) +
			checkYAML(sizedPartition("p", fmt.Sprintf("%q", brain)), ""),
			"is the name the model file " + brain + " takes"},
	}
	for _, c := range errorCases {
		t.Run(c.name+" is a config error", func(t *testing.T) {
			_, err := LoadConfig(writeConfigPath(t, c.yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}
}
