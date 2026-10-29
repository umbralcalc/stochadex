package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// expandedAlike runs a macros: config through the macro tier and its expansion
// through the single runtime, and checks they record exactly the same rows at
// exactly the same times.
func expandedAlike(t *testing.T, yaml string) *ApiRunConfig {
	t.Helper()
	path := writeConfigPath(t, yaml)
	// The expansion is made from a config of its own, loaded afresh and never
	// run: running a macros: config can write resolved values back into it,
	// which an expansion of the same object would then carry.
	fresh, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := Expand(fresh)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	macro, err := RunToStorage(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded.Macros) != 0 {
		t.Fatalf("the expansion still has macros: %v", expanded.Macros)
	}
	single, err := RunToStorage(expanded)
	if err != nil {
		t.Fatal(err)
	}
	assertStoragesEqual(t, "expanded vs macro tier", single.Storage, macro.Storage)
	if got, want := fmt.Sprint(single.Storage.GetTimes()), fmt.Sprint(macro.Storage.GetTimes()); got != want {
		t.Errorf("times differ")
	}
	return expanded
}

// aggregationConfig is a macros: config over a two-wide normal stream with
// the given macros.
func aggregationConfig(macros string) string {
	return `inputs:
  data:
    simulation:
      steps: 60
      timestep: 0.5
      init_time: 2.0
      partitions:
      - name: stream
        iteration: {type: data_generation, likelihood: {type: normal}}
        params: {mean: [1.0, -3.0], covariance_matrix: [2.0, 0.3, 0.3, 1.0]}
        init_state_values: [1.0, -3.0]
        state_history_depth: 1
        seed: 41
      - name: level
        iteration: {type: wiener_process}
        params: {variances: [0.5]}
        init_state_values: [4.0]
        state_history_depth: 1
        seed: 3
macros:
` + macros
}

func TestExpandAggregationMacros(t *testing.T) {
	t.Run("the shipped aggregation example runs alike expanded", func(t *testing.T) {
		expandedAlike(t, readFile(t, "../../cfg/example_macro_config.yaml"))
	})

	t.Run("mean, variance and covariance, with indices, defaults and the default kernel", func(t *testing.T) {
		expandedAlike(t, aggregationConfig(`- {type: vector_mean, name: m, data: {partition_name: stream, value_indices: [1]}, default_value: 2.5, window: 7}
- {type: vector_variance, name: v, mean: {partition_name: m}, data: {partition_name: stream, value_indices: [1]}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [3.0]}, default_value: 1.5, window: 7}
- {type: vector_covariance, name: c, mean: {partition_name: m}, data: {partition_name: stream, value_indices: [1]}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [3.0]}, default_value: 1.5, window: 7}
- {type: vector_mean, name: lm, data: {partition_name: level}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 4}
- {type: vector_covariance, name: lc, mean: {partition_name: lm}, data: {partition_name: level}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, default_value: 1.5, window: 4}
`))
	})

	t.Run("a macro reading an earlier macro's output over a window, and part of a wider mean", func(t *testing.T) {
		expandedAlike(t, aggregationConfig(`- {type: vector_mean, name: m, data: {partition_name: stream}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [3.0]}, window: 5}
- {type: vector_mean, name: mm, data: {partition_name: m}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 4}
- {type: vector_variance, name: v, mean: {partition_name: m, value_indices: [1]}, data: {partition_name: stream, value_indices: [1]}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [3.0]}, window: 5}
`))
	})

	t.Run("a two-wide covariance puts its default on the diagonal", func(t *testing.T) {
		expanded := expandedAlike(t, aggregationConfig(`- {type: vector_mean, name: m, data: {partition_name: stream}, window: 5}
- {type: vector_covariance, name: c, mean: {partition_name: m}, data: {partition_name: stream}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, default_value: 9.0, window: 5}
`))
		for _, partition := range expanded.Main.Partitions {
			if partition.Name == "c" && fmt.Sprint(partition.InitStateValues) != "[9 0 0 9]" {
				t.Errorf("covariance starts at %v, want [9 0 0 9]", partition.InitStateValues)
			}
		}
	})

	t.Run("a data: block expands to the input named data", func(t *testing.T) {
		yaml := inputsAsData(t, readFile(t, "../../cfg/example_macro_config.yaml"))
		expanded := expandedAlike(t, yaml)
		if _, ok := expanded.Inputs["data"]; !ok || expanded.Data != nil {
			t.Errorf("the expansion should declare inputs: {data: ...}, got inputs %v data %v",
				expanded.Inputs, expanded.Data)
		}
	})

	dir := t.TempDir()
	csv := filepath.Join(dir, "obs.csv")
	rows := []string{}
	for i := 0; i < 30; i++ {
		rows = append(rows, fmt.Sprintf("%d,%g,%g", i, float64(i%7)*0.5, float64((i*3)%5)-1))
	}
	if err := os.WriteFile(csv, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourced := fmt.Sprintf(`data:
  source: {csv: {path: %q, time_column: 0, state_columns: {obs: [1, 2]}}}
macros:
- {type: vector_mean, name: m, data: {partition_name: obs}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [4.0]}, window: 6}
`, csv)

	t.Run("a file source's shape is read from the file", func(t *testing.T) {
		expanded := expandedAlike(t, sourced)
		if expanded.Main.Simulation.InitTimeValue != 0 {
			t.Errorf("init time %v, want the file's first time 0", expanded.Main.Simulation.InitTimeValue)
		}
	})

	t.Run("several inputs: each partition replays from its own, clocked by the first", func(t *testing.T) {
		yaml := fmt.Sprintf(`inputs:
  alpha:
    source: {csv: {path: %q, time_column: 0, state_columns: {obs: [1, 2]}}}
  beta:
    source: {csv: {path: %q, time_column: 0, state_columns: {other: [2]}}}
macros:
- {type: vector_mean, name: m, data: {partition_name: obs, value_indices: [0]}, window: 3}
- {type: vector_mean, name: n, data: {partition_name: other}, window: 3}
`, csv, csv)
		expanded := expandedAlike(t, yaml)
		inputs := map[string]interface{}{}
		for _, partition := range expanded.Main.Partitions {
			if partition.IterationSpec.Type == "from_input" {
				inputs[partition.Name] = partition.IterationSpec.Fields["input"]
			}
		}
		if inputs["obs"] != "alpha" || inputs["other"] != "beta" {
			t.Errorf("replays read %v, want obs from alpha and other from beta", inputs)
		}
	})

	t.Run("the config's outputs are the expansion's, and write the same rows", func(t *testing.T) {
		macroLog, expandedLog := filepath.Join(dir, "macro.log"), filepath.Join(dir, "expanded.log")
		yaml := sourced + fmt.Sprintf("outputs:\n- {name: log, function: {type: json_log, path: %q}}\n", macroLog)
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunWith(config, WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}
		expandedYAML, err := ExpandToYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		expanded, err := LoadConfig(writeConfigPath(t, strings.Replace(string(expandedYAML), macroLog, expandedLog, 1)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunWith(expanded, WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}
		assertSameEntries(t, "the logs", keyedEntries(t, expandedLog), keyedEntries(t, macroLog))
	})

	t.Run("an expansion is written the same every time", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, readFile(t, "../../cfg/example_macro_config.yaml")))
		if err != nil {
			t.Fatal(err)
		}
		first, err := ExpandToYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		for range 5 {
			if again, _ := ExpandToYAML(config); string(again) != string(first) {
				t.Fatal("two expansions of one config differ")
			}
		}
	})

	cases := []struct{ name, yaml, want string }{
		{"a macro that does not expand yet",
			readFile(t, "../../cfg/example_smc_config.yaml"),
			`macro "smc_inference" does not expand yet`},
		{"a macro reading a partition nothing makes",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: missing}, window: 3}\n"),
			`macro "vector_mean": no partition "missing" to read (the inputs and earlier macros have: [level stream])`},
		{"a value index past a partition's width",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: level, value_indices: [1]}, window: 3}\n"),
			`value index 1 is outside partition "level"'s 1 values`},
		{"a mean index past its partition's width",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: level}, window: 3}\n" +
				"- {type: vector_variance, name: v, mean: {partition_name: m, value_indices: [2]}, data: {partition_name: level}, window: 3}\n"),
			`value index 2 is outside partition "m"'s 1 values`},
		{"two macros reading one partition over different windows",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: level}, window: 3}\n" +
				"- {type: vector_mean, name: n, data: {partition_name: level}, window: 5}\n"),
			`macros[1] (vector_mean) reads "level" over a window of 5, but macros[0] (vector_mean) reads it over 3`},
		{"a macro naming its partition as an input's",
			aggregationConfig("- {type: vector_mean, name: level, data: {partition_name: stream}, window: 3}\n"),
			`macros[0] (vector_mean) names its partition "level", which the inputs or an earlier macro already have`},
		{"a macro naming its partition as an earlier macro's",
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: stream}, window: 3}\n" +
				"- {type: vector_mean, name: m, data: {partition_name: level}, window: 2}\n"),
			`macros[1] (vector_mean) names its partition "m"`},
		{"a macro with nothing to read: no inputs, and no clock of its own",
			"macros:\n- {type: vector_mean, name: m, data: {partition_name: x}, window: 2}\n",
			"expanding needs the inputs the macros read, or a first macro that runs its own clock"},
	}
	for _, c := range cases {
		t.Run(c.name+" does not expand, saying why", func(t *testing.T) {
			config, err := LoadConfig(writeConfigPath(t, c.yaml))
			if err != nil {
				t.Fatal(err)
			}
			_, err = Expand(config)
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("an input that cannot be read stops the expansion, as unavailable", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, strings.Replace(sourced, csv, filepath.Join(dir, "gone.csv"), 1)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(config); KindOf(err) != ErrUnavailable ||
			!strings.Contains(err.Error(), `api: reading input "data"'s shape: `) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("an expansion that does not load says so", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t,
			aggregationConfig("- {type: vector_mean, name: m, data: {partition_name: stream}, kernel: {type: boxcar}, window: 3}\n")))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(config); err == nil ||
			!strings.Contains(err.Error(), "api: the expanded config does not load: ") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a config assembled in Go has no written inputs to expand", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, readFile(t, "../../cfg/example_macro_config.yaml")))
		if err != nil {
			t.Fatal(err)
		}
		assembled := &ApiRunConfig{Inputs: config.Inputs, Macros: config.Macros}
		if _, err := Expand(assembled); KindOf(err) != ErrConfig ||
			!strings.Contains(err.Error(), "a config assembled in Go has no source text") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("inputs on different time axes do not expand, as they do not run", func(t *testing.T) {
		shorter := filepath.Join(dir, "short.csv")
		if err := os.WriteFile(shorter, []byte(strings.Join(rows[:10], "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		config, err := LoadConfig(writeConfigPath(t, fmt.Sprintf(`inputs:
  alpha:
    source: {csv: {path: %q, time_column: 0, state_columns: {obs: [1, 2]}}}
  beta:
    source: {csv: {path: %q, time_column: 0, state_columns: {other: [2]}}}
macros:
- {type: vector_mean, name: m, data: {partition_name: obs}, window: 3}
`, csv, shorter)))
		if err != nil {
			t.Fatal(err)
		}
		_, expandErr := Expand(config)
		_, runErr := RunToStorage(config)
		if KindOf(expandErr) != ErrData || runErr == nil || !strings.Contains(expandErr.Error(), runErr.Error()) {
			t.Errorf("expanding should fail as running does: %v / %v", expandErr, runErr)
		}
		missing := strings.Replace(string(config.source), shorter, filepath.Join(dir, "gone.csv"), 1)
		config, err = LoadConfig(writeConfigPath(t, missing))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(config); KindOf(err) != ErrUnavailable ||
			!strings.Contains(err.Error(), `api: reading input "beta"'s times: `) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a config with no macros has nothing to expand", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, checkYAML(walkPartition("w", ""), "")))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(config); KindOf(err) != ErrConfig ||
			!strings.Contains(err.Error(), "the config has no macros: to expand") {
			t.Errorf("got %v", err)
		}
	})
}
