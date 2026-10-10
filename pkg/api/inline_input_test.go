package api

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// fromStorageTwinYAML is cfg/example_from_storage_config.yaml written with its
// data as an explicit inline input, read by from_input: what the shipped
// config's from_storage now means.
const fromStorageTwinYAML = `inputs:
  sun:
    source:
      inline:
        times: [0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0,
                13.0, 14.0, 15.0, 16.0, 17.0, 18.0, 19.0]
        partitions:
          clear_sky_driver: [[0.0], [5.0], [30.0], [120.0], [340.0], [610.0], [790.0], [880.0],
                             [860.0], [720.0], [500.0], [250.0], [90.0], [20.0], [2.0], [0.0],
                             [0.0], [0.0], [0.0], [0.0]]
main:
  partitions:
  - name: clear_sky_driver
    iteration: {type: from_input, input: sun}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  - name: site_power
    iteration:
      type: expression
      fields: [{name: p}]
      outputs: ["max(0.2 * irradiance, 0)"]
    params: {irradiance: [0.0]}
    params_from_upstream:
      irradiance: {upstream: clear_sky_driver, indices: [0]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 0
  simulation:
    output_condition: {type: every_step}
    output_function: {type: stdout}
    termination_condition: {type: number_of_steps, max_steps: 10}
    timestep_function: {type: from_input, input: sun}
    init_time_value: 0.0
`

var clearSky = []float64{0, 5, 30, 120, 340, 610, 790, 880, 860, 720, 500}

func TestInlineInputs(t *testing.T) {
	shipped := filepath.Join(repoRootPath(t), "cfg", "example_from_storage_config.yaml")

	t.Run("the shipped from_storage example is its inline-input twin, and the series' own values", func(t *testing.T) {
		desugared, err := LoadConfig(shipped)
		if err != nil {
			t.Fatal(err)
		}
		got, err := RunToStorage(desugared)
		if err != nil {
			t.Fatal(err)
		}
		twin, err := RunToStorage(writeConfig(t, fromStorageTwinYAML))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "shipped vs twin", got.Storage, twin.Storage)
		// Independently: the driver replays the series, the panel is 0.2 times it
		// within the step, and the clock replays the series' times.
		drivers, power := got.Storage.GetValues("clear_sky_driver"), got.Storage.GetValues("site_power")
		for step, irradiance := range clearSky {
			if drivers[step][0] != irradiance {
				t.Fatalf("step %d: driver %v, want %v", step, drivers[step][0], irradiance)
			}
			if step > 0 && power[step][0] != math.Max(0.2*irradiance, 0) {
				t.Fatalf("step %d: power %v, want %v", step, power[step][0], 0.2*irradiance)
			}
		}
		if !floats.Equal(got.Storage.GetTimes(), []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
			t.Errorf("times %v", got.Storage.GetTimes())
		}
	})

	t.Run("the shipped example prints what its twin prints", func(t *testing.T) {
		print := func(path string) []string {
			var err error
			printed := captureStdout(t, func() { err = Execute([]string{"stochadex", "--config", path}) })
			if err != nil {
				t.Fatal(err)
			}
			return sortedLines(printed)
		}
		if got, want := print(shipped), print(writeConfigPath(t, fromStorageTwinYAML)); !slices.Equal(got, want) {
			t.Errorf("printed\n%v\nwant\n%v", got, want)
		}
	})

	t.Run("a declared inline input drives partitions and the clock, and ends the run", func(t *testing.T) {
		yaml := `inputs:
  series:
    source:
      inline:
        times: [0.0, 0.5, 2.0, 2.25]
        partitions:
          a: [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0], [7.0, 8.0]]
          b: [[9.0], [8.0], [7.0], [6.0]]
main:
  partitions:
  - {name: a, iteration: {type: from_input, input: series}, state_history_depth: 1}
  - {name: renamed, iteration: {type: from_input, input: series, partition: b}, state_history_depth: 1}
  simulation:
    timestep_function: {type: from_input, input: series}
    termination_condition: {type: input_exhausted, input: series}
    init_time_value: 0.0
`
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][][]float64{
			"a":       {{1, 2}, {3, 4}, {5, 6}, {7, 8}},
			"renamed": {{9}, {8}, {7}, {6}},
		}
		for name, rows := range want {
			got := result.Storage.GetValues(name)
			if len(got) != len(rows) {
				t.Fatalf("%s: %d rows, want %d", name, len(got), len(rows))
			}
			for i := range rows {
				if !floats.Equal(got[i], rows[i]) {
					t.Fatalf("%s row %d: %v, want %v", name, i, got[i], rows[i])
				}
			}
		}
		if !floats.Equal(result.Storage.GetTimes(), []float64{0, 0.5, 2, 2.25}) {
			t.Errorf("times %v", result.Storage.GetTimes())
		}
	})

	t.Run("macros analyse inline data as they do the same data from a file", func(t *testing.T) {
		csv := filepath.Join(t.TempDir(), "data.csv")
		rows, inline := "", ""
		for step := range 40 {
			value := math.Sin(float64(step) / 3)
			rows += fmt.Sprintf("%d,%v\n", step, value)
			inline += fmt.Sprintf("[%v], ", value)
		}
		writeFileAt(t, csv, rows)
		times := make([]string, 40)
		for i := range times {
			times[i] = fmt.Sprint(i)
		}
		macro := `macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [5.0]}
  window: 10
`
		fromFile := fmt.Sprintf("data:\n  source:\n    csv: {path: %q, time_column: 0, state_columns: {stream: [1]}}\n", csv) + macro
		fromInline := fmt.Sprintf("data:\n  source:\n    inline:\n      times: [%s]\n      partitions: {stream: [%s]}\n",
			strings.Join(times, ", "), strings.TrimSuffix(inline, ", ")) + macro
		want, err := RunToStorage(writeConfig(t, fromFile))
		if err != nil {
			t.Fatal(err)
		}
		got, err := RunToStorage(writeConfig(t, fromInline))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "inline vs csv", got.Storage, want.Storage)
	})

	t.Run("inline data is in the config, so the config's digest fingerprints it", func(t *testing.T) {
		pinnedBuild(t, "pinned")
		key := func(yaml string) *Provenance {
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
		first := key(fromStorageTwinYAML)
		if first.Key == "" || first.Inputs[0].Kind != "inline" || first.Inputs[0].Fingerprint != "config" {
			t.Fatalf("provenance %+v", first)
		}
		if changed := key(strings.Replace(fromStorageTwinYAML, "[880.0]", "[881.0]", 1)); changed.Key == first.Key {
			t.Error("changing one inline value left the key unchanged")
		}
	})

	t.Run("from_storage with init_steps_taken, and an embedded run's, are left as written", func(t *testing.T) {
		data, err := os.ReadFile(shipped)
		if err != nil {
			t.Fatal(err)
		}
		yaml := strings.Replace(string(data), "      type: from_storage\n",
			"      type: from_storage\n      init_steps_taken: 0\n", 1)
		config, err := LoadConfig(writeConfigPath(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		if _, desugared := config.Inputs[fromStoragePrefix+"clear_sky_driver"]; desugared {
			t.Error("a from_storage with init_steps_taken was made an input")
		}
		if _, desugared := config.Inputs[fromStoragePrefix+"timestep_function"]; !desugared {
			t.Error("the clock, which sets only data, should still be an input")
		}
		hosted := checkYAML(walkPartition("a", "")+hostPartition, hostedRun)
		embedded := strings.Replace(hosted, "iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [5.0]",
			"iteration: {type: from_storage, data: [[5.0], [5.0], [5.0], [5.0]]}, init_state_values: [5.0]", 1)
		if embedded == hosted {
			t.Fatal("the embedded fixture did not change")
		}
		inner, err := LoadConfig(writeConfigPath(t, embedded))
		if err != nil {
			t.Fatal(err)
		}
		if len(inner.Inputs) != 0 {
			t.Errorf("an embedded run's from_storage was made an input: %v", inner.Inputs)
		}
	})

	errorCases := []struct{ name, source, want string }{
		{"rows that do not match the times", "{times: [0, 1, 2], partitions: {p: [[1], [2]]}}",
			`input "x": inline: partition "p" has 2 rows for 3 times`},
		{"a ragged row", "{times: [0, 1], partitions: {p: [[1, 2], [3]]}}",
			`partition "p"'s row 1 has 1 values, want 2 like its first`},
		{"no times", "{partitions: {p: [[1]]}}", "inline: needs times:"},
		{"an empty row", "{times: [0], partitions: {p: [[]]}}", `partition "p"'s row 0 has 0 values`},
	}
	for _, c := range errorCases {
		t.Run(c.name+" is a config error naming the input", func(t *testing.T) {
			yaml := "inputs:\n  x: {source: {inline: " + c.source + "}}\n" +
				checkYAML("  - {name: p, iteration: {type: from_input, input: x}, state_history_depth: 1}\n", "")
			_, err := LoadConfig(writeConfigPath(t, yaml))
			if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("an input already named as from_storage data would be is a config error", func(t *testing.T) {
		data, err := os.ReadFile(shipped)
		if err != nil {
			t.Fatal(err)
		}
		yaml := "inputs:\n  from_storage/clear_sky_driver: {source: {inline: {times: [0], partitions: {q: [[1]]}}}}\n" + string(data)
		_, err = LoadConfig(writeConfigPath(t, yaml))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(),
			`input "from_storage/clear_sky_driver" is the name inline from_storage data takes`) {
			t.Errorf("expected a naming error, got %v", err)
		}
	})
}
