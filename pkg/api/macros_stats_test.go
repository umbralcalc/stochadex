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
	// With-intercept cumulative layout is width 9:
	// [n, Sx, Sy, Sxx, Sxy, Syy, alpha, beta, sigma2] — so index 6 is the intercept
	// (alpha) and index 7 the slope (beta). Assert the SPECIFIC slots recover the
	// exact relation (not merely that 2.5 appears somewhere in the state), and that
	// the residual variance is ~0 for a noise-free line.
	if len(final) != 9 {
		t.Fatalf("expected width-9 with-intercept state, got %d: %v", len(final), final)
	}
	if got := final[7]; math.Abs(got-2.5) > 1e-6 {
		t.Errorf("slope (beta, index 7) = %v, want 2.5", got)
	}
	if got := final[6]; math.Abs(got-1.0) > 1e-6 {
		t.Errorf("intercept (alpha, index 6) = %v, want 1.0", got)
	}
	if got := final[8]; math.Abs(got) > 1e-6 {
		t.Errorf("residual variance (sigma2, index 8) = %v, want ~0 for a noise-free line", got)
	}
}

// TestScalarRegressionMacroRecoversNoisy guards the shipped example end-to-end:
// the regression macro recovers the slope (2.5) and intercept (1.0) of a linear
// relation observed through N(0, 1.5) noise, and estimates the residual variance
// (~1.5^2). This is the "does its job" bar — recovery from noise, not exact
// algebra on a clean line — matching the convergence tests for the other macros.
func TestScalarRegressionMacroRecoversNoisy(t *testing.T) {
	storage, err := runMacros(LoadApiRunConfigFromYaml(
		"../../cfg/example_regression_config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rows := storage.GetValues("ols")
	if len(rows) == 0 {
		t.Fatal("no ols output recorded")
	}
	final := rows[len(rows)-1]
	if len(final) != 9 {
		t.Fatalf("expected width-9 with-intercept state, got %d: %v", len(final), final)
	}
	if got := final[7]; math.Abs(got-2.5) > 0.1 {
		t.Errorf("slope (beta) = %v, want ~2.5", got)
	}
	if got := final[6]; math.Abs(got-1.0) > 0.2 {
		t.Errorf("intercept (alpha) = %v, want ~1.0", got)
	}
	if got := final[8]; math.Abs(got-2.25) > 0.75 {
		t.Errorf("residual variance (sigma2) = %v, want ~2.25 (1.5^2)", got)
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
