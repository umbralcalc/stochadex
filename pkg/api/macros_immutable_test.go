package api

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gopkg.in/yaml.v2"
)

// macroConfigsWithoutShippedExample are configs for the macro types no cfg/
// example or skill recipe ships, so every type in macroSpecFactories is run by
// TestMacroRunLeavesConfigUnmodified. Each exercises the maps its spec hands to
// the partitions it builds: params, params_as_partitions, params_from_upstream
// and, for the window, inner partitions with params of their own.
var macroConfigsWithoutShippedExample = map[string]string{
	"vector_covariance": `data:
  steps: 40
  timestep: 1.0
  partitions:
  - name: data_stream
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [1.8, 5.0], covariance_matrix: [2.5, 0.0, 0.0, 9.0]}
    init_state_values: [1.3, 8.3]
    state_history_depth: 20
    seed: 291
macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [10.0]}
  window: 15
- type: vector_covariance
  name: rolling_cov
  mean: {partition_name: rolling_mean}
  data: {partition_name: data_stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [10.0]}
  window: 15
  default_value: 3.0
`,
	"grouped_aggregation": `data:
  steps: 30
  timestep: 1.0
  partitions:
  - {name: vals, init_state_values: [0.0, 0.0, 0.0], state_history_depth: 2, seed: 0}
  - {name: grp, init_state_values: [1.0, 2.0, 1.0], state_history_depth: 2, seed: 0}
  expressions:
  - {partition: vals, fields: [{name: a}, {name: b}, {name: c}], outputs: ["t", "t + 1", "t + 2"]}
  - {partition: grp, fields: [{name: a}, {name: b}, {name: c}], outputs: ["1", "2", "1"]}
macros:
- type: grouped_aggregation
  name: agg
  aggregation: sum
  group_by: [{partition_name: grp}]
  precision: 1
  data: {partition_name: vals}
  window: 2
`,
	"likelihood_comparison": `data:
  steps: 40
  timestep: 1.0
  partitions:
  - name: test_data
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [2.0], covariance_matrix: [1.0]}
    init_state_values: [2.0]
    state_history_depth: 20
    seed: 123
  - name: model_mean
    iteration: {type: constant_values}
    init_state_values: [2.5]
    state_history_depth: 20
    seed: 0
macros:
- type: likelihood_comparison
  name: test_likelihood
  model:
    likelihood: {type: normal}
    params: {covariance_matrix: [1.0]}
    params_as_partitions: {reference_partition: [model_mean]}
    params_from_upstream:
      mean: {upstream: shifted_mean}
  data: {partition_name: test_data}
  window:
    partitions:
    - partition:
        name: shifted_mean
        iteration: {type: param_values}
        params: {param_values: [0.5]}
        params_as_partitions: {reference_partition: [model_mean]}
        init_state_values: [0.5]
        state_history_depth: 1
        seed: 0
    data: [{partition_name: test_data}, {partition_name: model_mean}]
    depth: 10
  window_data_history_depth: {test_data: 10, model_mean: 10}
`,
	"likelihood_mean_function_fit": `data:
  steps: 40
  timestep: 1.0
  partitions:
  - name: test_data
    iteration: {type: data_generation, likelihood: {type: normal}}
    params: {mean: [2.0, 3.0], covariance_matrix: [1.0, 0.0, 0.0, 1.0]}
    init_state_values: [2.0, 3.0]
    state_history_depth: 40
    seed: 5
macros:
- type: likelihood_mean_function_fit
  name: mean_fit
  model:
    likelihood: {type: normal}
    params: {covariance_matrix: [1.0, 0.0, 0.0, 1.0]}
    params_as_partitions: {reference_partition: [unit_covariance]}
    params_from_upstream:
      reference_values: {upstream: unit_covariance}
  gradient: {function: mean_gradient, width: 2}
  data: {partition_name: test_data}
  window:
    partitions:
    - partition:
        name: unit_covariance
        iteration: {type: param_values}
        params: {param_values: [1.0, 0.0, 0.0, 1.0]}
        init_state_values: [1.0, 0.0, 0.0, 1.0]
        state_history_depth: 1
        seed: 0
    depth: 10
  learning_rate: 0.005
  descent_iterations: 3
  window_data_history_depth: {test_data: 10}
`,
}

// marshalledMacroTiers is the config's data: and macros: tiers as YAML — what a
// macro run must leave as it found it.
func marshalledMacroTiers(t *testing.T, config *ApiRunConfig) string {
	t.Helper()
	out, err := yaml.Marshal(struct {
		Data   *DataConfig   `yaml:"data"`
		Macros []MacroConfig `yaml:"macros"`
	}{config.Data, config.Macros})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// sameStorage reports whether two storages hold the same partitions, times and
// values.
func sameStorage(a, b *simulator.StateTimeStorage) bool {
	if !floats.Equal(a.GetTimes(), b.GetTimes()) {
		return false
	}
	names := a.GetNames()
	if len(names) != len(b.GetNames()) {
		return false
	}
	for _, name := range names {
		av, bv := a.GetValues(name), b.GetValues(name)
		if len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !floats.Equal(av[i], bv[i]) {
				return false
			}
		}
	}
	return true
}

// TestMacroRunLeavesConfigUnmodified pins RunWith's and RunToStorage's "the
// caller's config is not modified" for the macros: tier, for every macro type:
// the data: and macros: tiers marshal byte-identically after a run, and running
// the same loaded config again gives what a freshly loaded one does. The specs
// hand their params / params_as_partitions / params_from_upstream maps to the
// partitions they build, which the pkg/macros constructors and the run then
// write resolved indices and upstream values into, so each must be copied.
func TestMacroRunLeavesConfigUnmodified(t *testing.T) {
	// Run from the repo root so a config's relative paths (e.g. a CSV source)
	// resolve as they do for the CLI.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	// A cleanup rather than a defer, so the parallel subtests run from the root.
	t.Cleanup(func() { os.Chdir(wd) })

	paths := map[string]string{}
	covered := map[string]bool{}
	for _, pattern := range []string{"cfg/*.yaml", ".claude/skills/stochadex-model/recipes/*.yaml"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range matches {
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Not every file there is a run config (cfg/ holds socket configs too).
			if !strings.Contains(string(text), "\nmacros:") {
				continue
			}
			config := LoadApiRunConfigFromYaml(path)
			if len(config.Macros) == 0 {
				continue
			}
			paths[path] = path
			for _, macro := range config.Macros {
				covered[macro.Type] = true
			}
		}
	}
	dir := t.TempDir()
	for macroType, text := range macroConfigsWithoutShippedExample {
		if covered[macroType] {
			t.Errorf("%s now ships an example: drop its config here", macroType)
		}
		path := filepath.Join(dir, macroType+".yaml")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[macroType] = path
		covered[macroType] = true
	}
	for macroType := range macroSpecFactories {
		if !covered[macroType] {
			t.Errorf("no config runs macro type %s: add one to macroConfigsWithoutShippedExample", macroType)
		}
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)

	runners := map[string]func(*ApiRunConfig) (*RunResult, error){
		"RunToStorage": RunToStorage,
		"RunWith":      func(config *ApiRunConfig) (*RunResult, error) { return RunWith(config) },
	}
	for _, name := range names {
		path := paths[name]
		for runnerName, run := range runners {
			t.Run(strings.TrimSuffix(filepath.Base(name), ".yaml")+"/"+runnerName, func(t *testing.T) {
				t.Parallel()
				config := LoadApiRunConfigFromYaml(path)
				before := marshalledMacroTiers(t, config)
				first, err := run(config)
				if err != nil {
					t.Fatal(err)
				}
				if after := marshalledMacroTiers(t, config); after != before {
					t.Fatalf("running the macros modified the config:\nbefore:\n%s\nafter:\n%s", before, after)
				}
				again, err := run(config)
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := run(LoadApiRunConfigFromYaml(path))
				if err != nil {
					t.Fatal(err)
				}
				if !sameStorage(first.Storage, fresh.Storage) {
					t.Error("first run of the loaded config differs from a freshly loaded one's")
				}
				if !sameStorage(again.Storage, fresh.Storage) {
					t.Error("running the same loaded config again differs from a freshly loaded one's")
				}
			})
		}
	}
}
