package api

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func runMacroConfig(t *testing.T, yamlText string) map[string][][]float64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0o644); err != nil {
		t.Fatal(err)
	}
	storage, err := runMacros(LoadApiRunConfigFromYaml(path))
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string][][]float64)
	for _, name := range storage.GetNames() {
		out[name] = storage.GetValues(name)
	}
	return out
}

// TestScalarRegressionMacro checks the regression macro recovers the slope of a
// deterministic linear relation y = 2.5x + 1 (and exercises the y/n YAML-boolean
// coercion fix, since the partitions are named x and y).
func TestScalarRegressionMacro(t *testing.T) {
	const cfg = `data:
  steps: 50
  timestep: 1.0
  partitions:
  - {name: x, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  - {name: y, init_state_values: [0.0], state_history_depth: 1, seed: 0}
  expressions:
  - {partition: x, fields: [{name: v}], outputs: ["t"]}
  - {partition: y, fields: [{name: v}], outputs: ["2.5 * t + 1.0"]}
macros:
- type: scalar_regression_stats
  name: ols
  y: {partition_name: y}
  x: {partition_name: x}
  intercept: true
  mode: cumulative
`
	out := runMacroConfig(t, cfg)
	rows := out["ols"]
	if len(rows) == 0 {
		t.Fatal("no ols output")
	}
	final := rows[len(rows)-1]
	// With-intercept cumulative state ends [..., beta, alpha? ]: the slope estimate
	// is one of the closed-form outputs; assert some element recovers 2.5.
	foundSlope := false
	for _, v := range final {
		if math.Abs(v-2.5) < 1e-6 {
			foundSlope = true
		}
	}
	if !foundSlope {
		t.Errorf("regression did not recover slope 2.5; final state = %v", final)
	}
}

// TestGroupedAggregationMacro checks the grouped aggregation macro runs and
// produces per-group output.
func TestGroupedAggregationMacro(t *testing.T) {
	const cfg = `data:
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
`
	out := runMacroConfig(t, cfg)
	if len(out["agg"]) == 0 {
		t.Fatal("grouped aggregation produced no output")
	}
	if w := len(out["agg"][len(out["agg"])-1]); w != 2 {
		t.Errorf("expected 2 groups, got width %d", w)
	}
}
