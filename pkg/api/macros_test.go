package api

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

const macroConfigYAML = `data:
  steps: 500
  timestep: 1.0
  partitions:
  - name: data_stream
    iteration: {type: data_generation, likelihood: {type: normal, allow_default_covariance_fallback: true}}
    params:
      mean: [1.8, 5.0]
      covariance_matrix: [2.5, 0.0, 0.0, 9.0]
    init_state_values: [1.3, 8.3]
    state_history_depth: 200
    seed: 291
macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [100.0]}
  window: 150
- type: vector_variance
  name: rolling_var
  mean: {partition_name: rolling_mean}
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [100.0]}
  window: 150
`

// TestMacroTierRunsInProcess is the macro-tier acceptance: a data: + macros:
// config builds storage, expands the aggregation macros against it (the variance
// macro referencing the mean macro's output), and runs — all data, no toolchain.
// The rolling mean recovers the stream's true mean.
func TestMacroTierRunsInProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(macroConfigYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	config := LoadApiRunConfigFromYaml(path)
	if len(config.Macros) != 2 || config.Data == nil {
		t.Fatalf("data: and macros: not loaded (%d macros, data=%v)", len(config.Macros), config.Data != nil)
	}
	storage, err := runMacros(config)
	if err != nil {
		t.Fatal(err)
	}
	mean := storage.GetValues("rolling_mean")
	if len(mean) == 0 {
		t.Fatal("no rolling_mean output")
	}
	final := mean[len(mean)-1]
	want := []float64{1.8, 5.0}
	for i := range want {
		if math.Abs(final[i]-want[i]) > 0.4 {
			t.Errorf("rolling_mean[%d] = %v, want ~%v", i, final[i], want[i])
		}
	}
	if len(storage.GetValues("rolling_var")) == 0 {
		t.Error("no rolling_var output (variance macro did not run against the mean macro)")
	}
}

func TestMacroErrors(t *testing.T) {
	t.Run("unknown macro type is rejected at decode", func(t *testing.T) {
		var config ApiRunConfig
		err := yaml.Unmarshal([]byte("macros:\n- type: nope\n"), &config)
		if err == nil {
			t.Error("expected an error decoding an unknown macro type")
		}
	})
	t.Run("macros without data", func(t *testing.T) {
		var config ApiRunConfig
		if err := yaml.Unmarshal(
			[]byte("macros:\n- {type: vector_mean, name: m, data: {partition_name: x}}\n"),
			&config,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := runMacros(&config); err == nil {
			t.Error("expected an error when macros have no data: block")
		}
	})
}

// TestRunMacrosGuards covers the macro-path validation: main+macros is a mistake,
// and a cyclic data: sub-simulation is caught before it can hang.
func TestRunMacrosGuards(t *testing.T) {
	t.Run("main partitions and macros together is rejected", func(t *testing.T) {
		config := &ApiRunConfig{
			Main:   RunConfig{Partitions: []simulator.PartitionConfig{{Name: "p"}}},
			Macros: []MacroConfig{{Type: "vector_mean", Spec: &vectorMeanSpec{}}},
		}
		if _, err := runMacros(config); err == nil {
			t.Error("expected an error when both main.partitions and macros: are set")
		}
	})

	t.Run("a cyclic data: sub-simulation is caught, not hung", func(t *testing.T) {
		config := &ApiRunConfig{
			Data: &DataConfig{
				Steps: 5,
				Partitions: []simulator.PartitionConfig{
					{
						Name:               "a",
						IterationSpec:      simulator.ComponentSpec{Type: "constant_values"},
						InitStateValues:    []float64{0.0},
						StateHistoryDepth:  1,
						ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "b"}},
					},
					{
						Name:               "b",
						IterationSpec:      simulator.ComponentSpec{Type: "constant_values"},
						InitStateValues:    []float64{0.0},
						StateHistoryDepth:  1,
						ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{"in": {Upstream: "a"}},
					},
				},
			},
			Macros: []MacroConfig{{Type: "vector_mean", Spec: &vectorMeanSpec{}}},
		}
		_, err := runMacros(config)
		if err == nil || !strings.Contains(err.Error(), "deadlock") {
			t.Errorf("expected a deadlock error from the cyclic data: block, got: %v", err)
		}
	})

	// Every key below is one the macros: context never reads, so each must be
	// rejected rather than silently ignored — and the error must name the key, so
	// the author knows what to move. Each case goes through the public path a real
	// config takes: a YAML file, LoadApiRunConfigFromYaml, then RunMacros.
	ignored := []struct {
		name, extra, key string
	}{
		{"main expressions",
			"main:\n  expressions:\n  - {partition: p, outputs: [\"1\"]}\n",
			"main.expressions"},
		{"main simulation",
			"main:\n  simulation:\n    output_function: {type: stdout}\n",
			"main.simulation"},
		{"main simulation init time only",
			"main:\n  simulation:\n    init_time_value: 5.0\n",
			"main.simulation"},
		{"embedded runs",
			"embedded:\n- name: e\n  partitions: []\n",
			"embedded:"},
		{"ensemble mode",
			"run: {mode: ensemble, seeds: [1, 2]}\n",
			"run:"},
		{"ensemble mode without seeds",
			"run: {mode: ensemble}\n",
			"run:"},
		{"seeds alone",
			"run: {seeds: [1]}\n",
			"run:"},
		{"concurrency alone",
			"run: {concurrency: 4}\n",
			"run:"},
	}
	for _, c := range ignored {
		t.Run(c.name+" alongside macros is rejected, naming the key", func(t *testing.T) {
			config := writeConfig(t, macroConfigYAML+c.extra)
			_, err := RunMacros(config)
			if err == nil {
				t.Fatalf("expected an error for %s alongside macros:", c.name)
			}
			if !strings.Contains(err.Error(), c.key) {
				t.Errorf("error should name %q so the author knows what to move: %v",
					c.key, err)
			}
		})
	}

	// Positive controls: the guard must not over-reject. The same config with no
	// extra keys, or with the explicit default run mode, still runs end to end
	// and produces both macros' output.
	accepted := map[string]string{
		"no extra keys":      "",
		"explicit run batch": "run: {mode: batch}\n",
	}
	for name, extra := range accepted {
		t.Run(name+" alongside macros still runs", func(t *testing.T) {
			config := writeConfig(t, macroConfigYAML+extra)
			storage, err := RunMacros(config)
			if err != nil {
				t.Fatalf("expected the config to run: %v", err)
			}
			for _, partition := range []string{"rolling_mean", "rolling_var"} {
				if len(storage.GetValues(partition)) == 0 {
					t.Errorf("expected %s in the output", partition)
				}
			}
		})
	}
}

// ensembleMainConfigYAML is a minimal main: config runnable in ensemble mode.
const ensembleMainConfigYAML = `main:
  partitions:
  - name: w
    iteration: {type: wiener_process}
    params: {variances: [1.0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 1
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    termination_condition: {type: number_of_steps, max_steps: 5}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
run: {mode: ensemble, seeds: [1, 2]}
`

// TestValidateMainContext covers the main-path guard: data: is only read by the
// macros: tier, so on a config without macros it must be rejected — through the
// public RunEnsembleToStorage path, with a positive control that the same config
// minus data: still runs.
func TestValidateMainContext(t *testing.T) {
	dataBlock := "data:\n  steps: 5\n  partitions: []\n"

	t.Run("data without macros is rejected, naming data:", func(t *testing.T) {
		config := writeConfig(t, ensembleMainConfigYAML+dataBlock)
		_, err := RunEnsembleToStorage(config)
		if err == nil {
			t.Fatal("expected RunEnsembleToStorage to reject data: without macros:")
		}
		if !strings.Contains(err.Error(), "data:") {
			t.Errorf("error should name data: so the author knows what to move: %v", err)
		}
	})

	t.Run("the same config without data still runs", func(t *testing.T) {
		config := writeConfig(t, ensembleMainConfigYAML)
		runs, err := RunEnsembleToStorage(config)
		if err != nil {
			t.Fatalf("expected the config to run: %v", err)
		}
		if len(runs) != 2 || len(runs[0].Storage.GetValues("w")) == 0 {
			t.Errorf("expected 2 members with recorded output, got %d", len(runs))
		}
	})
}
