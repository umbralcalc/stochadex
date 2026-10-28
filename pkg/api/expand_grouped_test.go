package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// groupedInputs steps values and two groupings: grp cycles through more
// groups over time, and band splits the values in two.
const groupedInputs = `inputs:
  data:
    simulation:
      steps: 40
      timestep: 1.0
      partitions:
      - {name: vals, init_state_values: [0.0, 1.0, 2.0], state_history_depth: 1, seed: 0}
      - {name: grp, init_state_values: [0.0, 0.0, 1.0], state_history_depth: 1, seed: 0}
      - {name: band, init_state_values: [0.0, 1.0, 0.0], state_history_depth: 1, seed: 0}
      expressions:
      - {partition: vals, fields: [{name: a}, {name: b}, {name: c}], outputs: ["t", "2 * t + 1", "t * t"]}
      - {partition: grp, fields: [{name: a}, {name: b}, {name: c}], outputs: ["floor(t / 10)", "floor(t / 15)", "1"]}
      - {partition: band, fields: [{name: a}, {name: b}, {name: c}], outputs: ["t - 2 * floor(t / 2)", "1", "0"]}
macros:
`

// groupedRows runs a config through its expansion and returns a partition's
// rows.
func groupedRows(t *testing.T, yaml, partition string) [][]float64 {
	t.Helper()
	expanded := expandedAlike(t, yaml)
	result, err := RunToStorage(expanded)
	if err != nil {
		t.Fatal(err)
	}
	return result.Storage.GetValues(partition)
}

func TestExpandGroupedAggregation(t *testing.T) {
	t.Run("groups appearing over time, discovered as the macro discovers them", func(t *testing.T) {
		expandedAlike(t, readFile(t, "testdata/grouped/grouped_macro.yaml"))
	})

	t.Run("declared groups are used in the order given", func(t *testing.T) {
		discovered := groupedRows(t, groupedInputs+
			"- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, window: 3}\n", "g")
		reversed := groupedRows(t, groupedInputs+
			"- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, window: 3, groups: [[3], [2], [1], [0]]}\n", "g")
		// Discovery finds 0, 1, 2, 3 in that order, so the reversed
		// declaration gives the same sums, reversed.
		for step := range discovered {
			want := append([]float64(nil), discovered[step]...)
			for i, j := 0, len(want)-1; i < j; i, j = i+1, j-1 {
				want[i], want[j] = want[j], want[i]
			}
			if !floats.Equal(reversed[step], want) {
				t.Fatalf("step %d: declared groups gave %v, want discovered %v reversed", step, reversed[step], discovered[step])
			}
		}
	})

	t.Run("grouping by two partitions, part of the data, a default, a kernel and params", func(t *testing.T) {
		expandedAlike(t, groupedInputs+`- type: grouped_aggregation
  name: g
  aggregation: mean
  group_by: [{partition_name: grp, value_indices: [0, 1]}, {partition_name: band, value_indices: [0, 1]}]
  precision: 0
  data: {partition_name: vals, value_indices: [0, 1]}
  default_value: -1.0
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [2.0]}
  window: 5
`)
	})

	t.Run("other gathers the undeclared groups into the sum of their own columns", func(t *testing.T) {
		all := groupedRows(t, groupedInputs+
			"- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, window: 3}\n", "g")
		gathered := groupedRows(t, groupedInputs+
			"- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, window: 3, groups: [[0], [1]], undeclared_groups: other}\n", "g")
		for step := 3; step < len(all); step++ {
			// Groups 2 and 3 are the undeclared ones; a sum over both is the
			// sum of their sums.
			want := []float64{all[step][0], all[step][1], all[step][2] + all[step][3]}
			if !floats.EqualApprox(gathered[step], want, 1e-9) {
				t.Fatalf("step %d: %v, want %v", step, gathered[step], want)
			}
		}
	})

	t.Run("error fails the expanded run at the first undeclared group, naming it", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, groupedInputs+
			"- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, window: 3, groups: [[0], [1]], undeclared_groups: error}\n"))
		if err != nil {
			t.Fatal(err)
		}
		expanded, err := Expand(config)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := RunToStorage(expanded); KindOf(err) != ErrRuntime || !strings.Contains(err.Error(), "group (2.0) is not declared") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("groups discovered in a file input", func(t *testing.T) {
		csv := filepath.Join(t.TempDir(), "g.csv")
		rows := []string{}
		for i := 0; i < 20; i++ {
			rows = append(rows, fmt.Sprintf("%d,%d,%g", i, (i*7)%3, float64(i)*0.5))
		}
		if err := os.WriteFile(csv, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		expandedAlike(t, fmt.Sprintf(`inputs:
  obs:
    source: {csv: {path: %q, time_column: 0, state_columns: {kind: [1], level: [2]}}}
macros:
- {type: grouped_aggregation, name: g, aggregation: count, group_by: [{partition_name: kind}], precision: 0, data: {partition_name: level}, window: 4}
`, csv))
	})

	t.Run("params override what the macro sets", func(t *testing.T) {
		expandedAlike(t, groupedInputs+
			"- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp, value_indices: [0]}], precision: 0, data: {partition_name: vals, value_indices: [0]}, window: 2, params: {state_value_indices: [2.0]}}\n")
	})

	t.Run("no window reads one step back, as a window of 1 does", func(t *testing.T) {
		expandedAlike(t, groupedInputs+`- {type: vector_mean, name: m, data: {partition_name: vals}, window: 1}
- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}}
`)
	})

	t.Run("grouping by an earlier macro's output, with its groups declared", func(t *testing.T) {
		expandedAlike(t, groupedInputs+`- {type: vector_mean, name: m, data: {partition_name: band}, window: 2}
- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: m, value_indices: [0]}], precision: 0, data: {partition_name: vals, value_indices: [0]}, window: 2, groups: [[0], [1]]}
`)
	})

	cases := []struct{ name, macro, want string }{
		{"an unknown aggregation", "{type: grouped_aggregation, name: g, aggregation: median, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}}",
			`grouped_aggregation: unknown aggregation "median"`},
		{"no group_by", "{type: grouped_aggregation, name: g, aggregation: sum, precision: 1, data: {partition_name: vals}}",
			"grouped_aggregation needs group_by"},
		{"an unknown undeclared-groups policy", "{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, undeclared_groups: ignore}",
			`undeclared_groups must be drop, error or other, got "ignore"`},
		{"a declared group of the wrong length", "{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, groups: [[0, 1]]}",
			"groups[0] has 2 values, but there are 1 group_by references"},
		{"a declared group with too few values", "{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}, {partition_name: band}], precision: 1, data: {partition_name: vals}, groups: [[0]]}",
			"groups[0] has 1 values, but there are 2 group_by references"},
		{"a grouping reading a different number of values from the data", "{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp, value_indices: [0]}], precision: 1, data: {partition_name: vals}, groups: [[0]]}",
			`group_by "grp" reads 1 values, but the data reads 3; each grouping value groups the data value at its position`},
		{"data nothing makes", "{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: gone}}",
			`no partition "gone" to read`},
		{"a grouping nothing makes", "{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: gone}], precision: 1, data: {partition_name: vals}, groups: [[0]]}",
			`no partition "gone" to read`},
		{"discovering groups in an earlier macro's output", "{type: vector_mean, name: m, data: {partition_name: band}, window: 2}\n- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: m}], precision: 0, data: {partition_name: vals}}",
			`grouped_aggregation groups by "m", which no input supplies, so it has no recorded values to discover groups in; declare them with groups:`},
	}
	for _, c := range cases {
		t.Run(c.name+" does not expand, saying why", func(t *testing.T) {
			config, err := LoadConfig(writeConfigPath(t, groupedInputs+"- "+c.macro+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Expand(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), c.want) {
				t.Errorf("expected ErrConfig containing %q, got %v", c.want, err)
			}
		})
	}

	t.Run("the macro tier refuses what the expansion refuses of the new fields", func(t *testing.T) {
		for macro, want := range map[string]string{
			"{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, undeclared_groups: ignore}": `undeclared_groups must be drop, error or other, got "ignore"`,
			"{type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: grp}], precision: 1, data: {partition_name: vals}, groups: [[0, 1]]}":          "groups[0] has 2 values",
		} {
			config, err := LoadConfig(writeConfigPath(t, groupedInputs+"- "+macro+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := RunToStorage(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), want) {
				t.Errorf("expected ErrConfig containing %q, got %v", want, err)
			}
		}
	})

	t.Run("an input that cannot be built stops discovery, saying why", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, `inputs:
  data:
    simulation:
      steps: 3
      partitions:
      - {name: x, iteration: {type: param_values}, params: {param_values: [0.0]}, params_from_upstream: {param_values: {upstream: y}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
      - {name: y, iteration: {type: param_values}, params: {param_values: [0.0]}, params_from_upstream: {param_values: {upstream: x}}, init_state_values: [0.0], state_history_depth: 1, seed: 0}
macros:
- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: x}], precision: 0, data: {partition_name: y}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(config); KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "deadlock") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("an input that cannot be read stops discovery", func(t *testing.T) {
		config, err := LoadConfig(writeConfigPath(t, `inputs:
  a: {source: {csv: {path: gone.csv, time_column: 0, state_columns: {kind: [1]}}}}
  b: {simulation: {steps: 2, timestep: 1.0, partitions: [{name: x, iteration: {type: constant_values}, params: {}, init_state_values: [1.0], state_history_depth: 1, seed: 0}]}}
macros:
- {type: grouped_aggregation, name: g, aggregation: sum, group_by: [{partition_name: x}], precision: 0, data: {partition_name: x}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Expand(config); KindOf(err) != ErrUnavailable {
			t.Errorf("got %v", err)
		}
	})
}
