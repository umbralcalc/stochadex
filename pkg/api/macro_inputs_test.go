package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataAsInputs rewrites a config's data: block as an equivalent inputs: entry —
// a data: source as a source input, a data: sub-simulation as a simulation
// input. It moves the block as text, re-indented, rather than re-encoding the
// document: a YAML round trip through interface{} turns a bare y into true.
func dataAsInputs(t *testing.T, contents string) string {
	t.Helper()
	lines := strings.Split(contents, "\n")
	start := -1
	for i, line := range lines {
		if line == "data:" {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("config has no top-level data: block to rewrite")
	}
	end := start + 1
	for end < len(lines) {
		line := lines[end]
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#") {
			break
		}
		end++
	}
	block := lines[start+1 : end]
	isSource := false
	for _, line := range block {
		if strings.HasPrefix(line, "  source:") {
			isSource = true
		}
	}
	indent, header := "  ", []string{"inputs:", "  observed:"}
	if !isSource {
		indent, header = "    ", []string{"inputs:", "  observed:", "    simulation:"}
	}
	moved := append([]string{}, header...)
	for _, line := range block {
		if strings.TrimSpace(line) == "" {
			moved = append(moved, line)
			continue
		}
		moved = append(moved, indent+line)
	}
	out := append(append(append([]string{}, lines[:start]...), moved...), lines[end:]...)
	return strings.Join(out, "\n")
}

// TestDataBlocksAsInputs is PLAN.md item 1.1's acceptance test: every shipped
// config that analyses a data: block — the cfg/ examples and the agent skill's
// recipes, covering csv sources, sub-simulations, against-storage and live
// macros — gives byte-identical results with that block moved into inputs:.
func TestDataBlocksAsInputs(t *testing.T) {
	wd, _ := os.Getwd()
	if err := os.Chdir("../.."); err != nil { // configs carry repo-relative paths
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	scratch := t.TempDir()

	paths := []string{}
	for _, pattern := range []string{"cfg/*.yaml", ".claude/skills/stochadex-model/recipes/*.yaml"} {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			if strings.Contains(readFile(t, path), "\ndata:") || strings.HasPrefix(readFile(t, path), "data:") {
				paths = append(paths, path)
			}
		}
	}
	if len(paths) < 9 {
		t.Fatalf("expected the 6 cfg examples and 3 recipes that use data:, found %v", paths)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			original, err := RunToStorage(LoadApiRunConfigFromYaml(path))
			if err != nil {
				t.Fatal(err)
			}
			rewritten := filepath.Join(scratch, strings.ReplaceAll(path, "/", "_"))
			writeFile(t, rewritten, dataAsInputs(t, readFile(t, path)))
			asInputs, err := RunToStorage(LoadApiRunConfigFromYaml(rewritten))
			if err != nil {
				t.Fatal(err)
			}
			assertSameStorage(t, "inputs: vs data:", asInputs.Storage, original.Storage)
			assertSameStorage(t, "data: vs inputs:", original.Storage, asInputs.Storage)
		})
	}
}

// twoInputsYAML is the rolling-mean macro over two partitions that come from two
// inputs: a CSV ("price") and a sub-simulation ("demand"), both on times 0..5.
func twoInputsYAML(csvPath, demandSteps string) string {
	return fmt.Sprintf(`inputs:
  prices:
    source: {csv: {path: %q, time_column: 0, state_columns: {price: [1]}}}
  demand:
    simulation:
      steps: %s
      partitions:
      - {name: demand, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [10.0], state_history_depth: 3, seed: 4}
macros:
- {type: vector_mean, name: price_mean, data: {partition_name: price}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 3}
- {type: vector_mean, name: demand_mean, data: {partition_name: demand}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 3}
`, csvPath, demandSteps)
}

func TestMacrosOverSeveralInputs(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "prices.csv")
	writeFile(t, csvPath, "0,1\n1,3\n2,2\n3,5\n4,4\n5,6\n")

	t.Run("each macro's result matches a run over its input alone", func(t *testing.T) {
		combined, err := RunToStorage(writeConfig(t, twoInputsYAML(csvPath, "5")))
		if err != nil {
			t.Fatal(err)
		}
		// References: each macro over its own input as the config's only data.
		priceOnly, err := RunToStorage(writeConfig(t, fmt.Sprintf(`data:
  source: {csv: {path: %q, time_column: 0, state_columns: {price: [1]}}}
macros:
- {type: vector_mean, name: price_mean, data: {partition_name: price}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 3}
`, csvPath)))
		if err != nil {
			t.Fatal(err)
		}
		demandOnly, err := RunToStorage(writeConfig(t, `data:
  steps: 5
  partitions:
  - {name: demand, iteration: {type: wiener_process}, params: {variances: [1.0]}, init_state_values: [10.0], state_history_depth: 3, seed: 4}
macros:
- {type: vector_mean, name: demand_mean, data: {partition_name: demand}, kernel: {type: exponential}, params: {exponential_weighting_timescale: [2.0]}, window: 3}
`))
		if err != nil {
			t.Fatal(err)
		}
		assertSameStorage(t, "price side", storageWith(combined.Storage, "price_mean"),
			storageWith(priceOnly.Storage, "price_mean"))
		assertSameStorage(t, "demand side", storageWith(combined.Storage, "demand_mean"),
			storageWith(demandOnly.Storage, "demand_mean"))
	})

	t.Run("inputs on different time axes fail the run, naming them", func(t *testing.T) {
		_, err := RunToStorage(writeConfig(t, twoInputsYAML(csvPath, "7")))
		if KindOf(err) != ErrData || !strings.Contains(err.Error(), `inputs "demand" and "prices"`) {
			t.Errorf("expected ErrData naming both inputs, got %v", err)
		}
	})

	t.Run("inputs with the same length but different times fail the run, naming the row", func(t *testing.T) {
		// Both have 6 rows, but demand is sampled every 2.0 rather than every 1.0.
		yaml := strings.Replace(twoInputsYAML(csvPath, "5"), "      steps: 5\n", "      steps: 5\n      timestep: 2.0\n", 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrData || !strings.Contains(err.Error(), "row 1: 2 vs 1") {
			t.Errorf("expected ErrData naming the first differing row, got %v", err)
		}
	})

	t.Run("a partition name in two inputs fails the run, naming both", func(t *testing.T) {
		yaml := strings.Replace(twoInputsYAML(csvPath, "5"), "state_columns: {price: [1]}", "state_columns: {demand: [1]}", 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrData || !strings.Contains(err.Error(), `partition "demand" is in both input`) {
			t.Errorf("expected ErrData naming the shared partition, got %v", err)
		}
	})

	t.Run("a missing input in a macros config is unavailable", func(t *testing.T) {
		yaml := strings.Replace(twoInputsYAML(csvPath, "5"), csvPath, filepath.Join(dir, "absent.csv"), 1)
		_, err := RunToStorage(writeConfig(t, yaml))
		if KindOf(err) != ErrUnavailable || !strings.Contains(err.Error(), `input "prices"`) {
			t.Errorf("expected ErrUnavailable naming the input, got %v", err)
		}
	})
}
