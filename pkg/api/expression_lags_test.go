package api

import (
	"fmt"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// lagYAML is docs/reference.md's history example: delayed reads counter (which
// counts 1, 2, 3, ...) through output, with counter keeping depth rows.
func lagYAML(depth int, output string) string {
	return fmt.Sprintf(`main:
  partitions:
  - name: counter
    iteration: {type: expression, fields: [{name: count}], outputs: ["count + 1"]}
    init_state_values: [0]
    state_history_depth: %d
    seed: 0
  - name: delayed
    iteration: {type: expression, fields: [{name: x}], upstreams: {c: counter}, outputs: [%q]}
    init_state_values: [-1]
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 4}
    timestep_function: {type: constant, stepsize: 1}
    init_time_value: 0
`, depth, output)
}

func TestLiteralLagsAtLoad(t *testing.T) {
	refused := func(t *testing.T, yaml, want string) {
		t.Helper()
		_, err := LoadConfig(writeConfigPath(t, yaml))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), want) {
			t.Errorf("expected a config error containing %q, got %v", want, err)
		}
	}

	t.Run("an upstream lag reads its rows when the upstream keeps them", func(t *testing.T) {
		result, err := RunToStorage(mustLoad(t, writeConfigPath(t, lagYAML(3, "lag(c, 2)[0]"))))
		if err != nil {
			t.Fatal(err)
		}
		// docs/reference.md's output for this example.
		want := [][]float64{{-1}, {0}, {0}, {0}, {1}}
		got := result.Storage.GetValues("delayed")
		for i := range want {
			if !floats.Equal(got[i], want[i]) {
				t.Fatalf("delayed = %v, want %v", got, want)
			}
		}
	})

	t.Run("an upstream lag past the rows its upstream keeps is refused, naming that partition", func(t *testing.T) {
		refused(t, lagYAML(2, "lag(c, 2)[0]"), `main expression partition "delayed": lag(c, 2) reads `+
			`2 rows back, but partition "counter" keeps 2 (state_history_depth: 2); raise its `+
			`state_history_depth to at least 3`)
	})

	t.Run("a lag behind a branch the run reaches late is refused as the run would refuse it", func(t *testing.T) {
		yaml := lagYAML(2, "where(step > 3, lag(c, 2)[0], -1)")
		refused(t, yaml, "lag(c, 2) reads 2 rows back")
		checked, _ := executeIn(t, t.TempDir(), "--config", writeConfigPath(t, yaml), "--check")
		if KindOf(checked) != ErrConfig || !strings.Contains(checked.Error(), "lag(c, 2) reads 2 rows back") {
			t.Errorf("--check gave %v", checked)
		}
	})

	t.Run("a lag of its own field reads its own history", func(t *testing.T) {
		own := strings.Replace(lagYAML(1, "lag(x, 1)[0] + 1"), "    init_state_values: [-1]\n    state_history_depth: 1",
			"    init_state_values: [-1]\n    state_history_depth: 2", 1)
		if _, err := LoadConfig(writeConfigPath(t, own)); err != nil {
			t.Errorf("a lag within the partition's own rows was refused: %v", err)
		}
		refused(t, lagYAML(1, "lag(x, 1)[0] + 1"),
			`lag(x, 1) reads 1 rows back, but partition "delayed" keeps 1`)
	})

	t.Run("a negative row is refused", func(t *testing.T) {
		refused(t, lagYAML(3, "lag(c, -1)[0]"), "lag(c, -1) reads row -1, but rows count back from 0")
	})

	t.Run("an alias naming no partition gets the wiring's error, not a depth", func(t *testing.T) {
		yaml := strings.Replace(lagYAML(3, "lag(c, 2)[0]"), "upstreams: {c: counter}", "upstreams: {c: ghost}", 1)
		checked, _ := executeIn(t, t.TempDir(), "--config", writeConfigPath(t, yaml), "--check")
		if KindOf(checked) != ErrConfig || !strings.Contains(checked.Error(), "upstream partition ghost (alias c) not found") {
			t.Errorf("--check gave %v", checked)
		}
	})

	t.Run("a lag of a name that is neither an alias nor a field is left to the run", func(t *testing.T) {
		_, err := RunToStorage(mustLoad(t, writeConfigPath(t, lagYAML(3, "lag(nope, 1)[0]"))))
		if err == nil || !strings.Contains(err.Error(), "lag needs an upstream alias or one of this partition's own fields, got nope") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a computed row is left to the run", func(t *testing.T) {
		if _, err := RunToStorage(mustLoad(t, writeConfigPath(t, lagYAML(3, "lag(c, min(step, 2))[0]")))); err != nil {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a lag in an embedded run is checked against its own run", func(t *testing.T) {
		hosted := strings.Replace(hostedRun, "  simulation:\n",
			"  - {name: echo, iteration: {type: expression, fields: [{name: e}], upstreams: {i: inner}, "+
				"outputs: [\"lag(i, 1)[0]\"]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n"+
				"  simulation:\n", 1)
		refused(t, checkYAML(walkPartition("a", "")+hostPartition, hosted),
			`embedded run "nested": expression partition "echo": lag(i, 1) reads 1 rows back, but partition "inner" keeps 1`)
	})

	t.Run("a lag in a nested run is checked against that run", func(t *testing.T) {
		nested := strings.Replace(nestedNamesRun("rate", "rate"), `outputs: ["x + 1"]`,
			`outputs: ["lag(x, 1)[0] + 1"]`, 1)
		refused(t, nested, `partition "host"'s nested run: expression partition "inner": `+
			`lag(x, 1) reads 1 rows back, but partition "inner" keeps 1`)
	})

	t.Run("a main partition a macro deepens is checked once the expansion has deepened it", func(t *testing.T) {
		const mean = "- {type: vector_mean, name: m, data: {partition_name: walk}, window: 5}\n"
		lagger := "  - {name: lagger, iteration: {type: expression, fields: [{name: l}], upstreams: {w: walk}, " +
			"outputs: [\"lag(w, 4)[0]\"]}, init_state_values: [0.0], state_history_depth: 1, seed: 0}\n"
		result := composedRun(t, walkMain(lagger, mean)).Storage
		walk, lagged := result.GetValues("walk"), result.GetValues("lagger")
		// Row 4 back from the last committed state is the walk five steps earlier.
		for step := 5; step < len(walk); step++ {
			if !floats.Equal(lagged[step], walk[step-5]) {
				t.Fatalf("step %d: lagger %v, want walk five steps back %v", step, lagged[step], walk[step-5])
			}
		}
		// One row past the deepened history: the config loads, and its expansion is refused.
		_, err := RunToStorage(writeConfig(t, walkMain(strings.Replace(lagger, "lag(w, 4)", "lag(w, 5)", 1), mean)))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(),
			`lag(w, 5) reads 5 rows back, but partition "walk" keeps 5`) {
			t.Errorf("expected the expansion to be refused, got %v", err)
		}
	})
}
