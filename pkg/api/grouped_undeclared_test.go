package api

import (
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// The series grouped: values that are distinct powers of two, so every sum
// says which rows went into it, in groups 1 and 2 (declared) and 3 and 4 (not).
var (
	groupedValues = []float64{1, 2, 4, 8, 16, 32, 64, 128}
	groupedGroups = []float64{1, 3, 2, 4, 1, 3, 2, 4}
)

// groupedYAML sums the values by group over a window of the latest row and
// the two before it, with groups 1 and 2 declared, spec fields extra and an
// output width.
func groupedYAML(extra, width string) string {
	return `inputs:
  series: {source: {inline: {times: [0, 1, 2, 3, 4, 5, 6, 7], partitions: {
    vals: [[1], [2], [4], [8], [16], [32], [64], [128]],
    grp: [[1], [3], [2], [4], [1], [3], [2], [4]]}}}}
main:
  partitions:
  - {name: vals, iteration: {type: from_input, input: series}, state_history_depth: 2, seed: 0}
  - {name: grp, iteration: {type: from_input, input: series}, state_history_depth: 2, seed: 0}
  - name: agg
    iteration: {type: values_grouped_aggregation, aggregation: sum, kernel: {type: constant}` + extra + `}
    params:
      accepted_value_group_tupindex_0: [1, 2]
      default_values: ` + width + `
      float_precision: [1]
      grouping_value_indices_tupindex_0: [0]
      state_value_indices: [0]
    params_as_partitions: {grouping_partition_tupindex_0: [grp], state_partition: [vals]}
    params_from_upstream: {latest_groupings_tupindex_0: {upstream: grp}, latest_states: {upstream: vals}}
    init_state_values: ` + width + `
    state_history_depth: 1
    seed: 0
  simulation:
    termination_condition: {type: number_of_steps, max_steps: 7}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`
}

// groupedSums is the reference: at step k, the sum of the window's values
// (rows k-2 to k) in each group that keep says to keep, gathered by bucket.
func groupedSums(step int, bucket func(group float64) int, width int) []float64 {
	sums := make([]float64, width)
	for row := step - 2; row <= step; row++ {
		if index := bucket(groupedGroups[row]); index >= 0 {
			sums[index] += groupedValues[row]
		}
	}
	return sums
}

func declaredBucket(group float64) int {
	switch group {
	case 1:
		return 0
	case 2:
		return 1
	}
	return -1
}

func TestUndeclaredGroups(t *testing.T) {
	run := func(t *testing.T, yaml string) [][]float64 {
		t.Helper()
		result, err := RunToStorage(writeConfig(t, yaml))
		if err != nil {
			t.Fatal(err)
		}
		return result.Storage.GetValues("agg")
	}
	// From step 2 the window holds only real rows.
	check := func(t *testing.T, rows [][]float64, bucket func(float64) int, width int) {
		t.Helper()
		for step := 2; step < len(rows); step++ {
			if want := groupedSums(step, bucket, width); !floats.Equal(rows[step], want) {
				t.Errorf("step %d: %v, want %v", step, rows[step], want)
			}
		}
	}

	t.Run("by default the declared groups are an accept-list and the rest are dropped", func(t *testing.T) {
		check(t, run(t, groupedYAML("", "[0, 0]")), declaredBucket, 2)
		check(t, run(t, groupedYAML(", undeclared_groups: drop", "[0, 0]")), declaredBucket, 2)
	})

	t.Run("other gathers every undeclared group into one more output", func(t *testing.T) {
		rows := run(t, groupedYAML(", undeclared_groups: other", "[0, 0, 0]"))
		check(t, rows, func(group float64) int {
			if index := declaredBucket(group); index >= 0 {
				return index
			}
			return 2
		}, 3)
		if rows[3][2] != 2+8 {
			t.Errorf("step 3's other is %v, want groups 3 and 4 together, 2+8", rows[3][2])
		}
	})

	t.Run("error checks each step's latest rows, not the initial row in the window", func(t *testing.T) {
		// Every group the series has is declared, but the grouping partition
		// starts in group 9, which the window holds for the first steps.
		yaml := strings.NewReplacer(
			"accepted_value_group_tupindex_0: [1, 2]", "accepted_value_group_tupindex_0: [1, 2, 3, 4]",
			"{name: grp, iteration: {type: from_input, input: series},",
			"{name: grp, iteration: {type: from_input, input: series}, init_state_values: [9.0],",
		).Replace(groupedYAML(", undeclared_groups: error", "[0, 0, 0, 0]"))
		if _, err := RunToStorage(writeConfig(t, yaml)); err != nil {
			t.Errorf("a run whose rows' groups are all declared failed: %v", err)
		}
		// The control: gathered as other, group 9 is in step 1's window.
		other := run(t, strings.NewReplacer("undeclared_groups: error", "undeclared_groups: other",
			"[0, 0, 0, 0]", "[0, 0, 0, 0, 0]").Replace(yaml))
		if other[1][4] == 0 {
			t.Error("group 9 was not in the window, so the run above proves nothing")
		}
	})

	t.Run("other with an output not one wider than the declared groups fails, saying why", func(t *testing.T) {
		for _, width := range []string{"[0, 0]", "[0, 0, 0, 0]"} {
			_, err := RunToStorage(writeConfig(t, groupedYAML(", undeclared_groups: other", width)))
			if err == nil || !strings.Contains(err.Error(), `partition "agg" declares 2 groups, so with `+
				`undeclared groups gathered as "other" its state is 3 wide, not `) {
				t.Errorf("width %s: got %v", width, err)
			}
		}
	})

	t.Run("other gathers groups in key order, so its value does not depend on map order", func(t *testing.T) {
		// In key order (groups 3, 4, 5) the sum is (1e16 + 1) - 1e16 = 0 in
		// float64; any other order gives 1.
		yaml := strings.NewReplacer(
			"vals: [[1], [2], [4], [8], [16], [32], [64], [128]]", "vals: [[1e16], [1], [-1e16], [0], [0], [0], [0], [0]]",
			"grp: [[1], [3], [2], [4], [1], [3], [2], [4]]", "grp: [[3], [4], [5], [1], [1], [1], [1], [1]]",
		).Replace(groupedYAML(", undeclared_groups: other", "[0, 0, 0]"))
		for attempt := 0; attempt < 100; attempt++ {
			if other := run(t, yaml)[2][2]; other != 0 {
				t.Fatalf("attempt %d: other is %v, want 0 from summing in key order", attempt, other)
			}
		}
	})

	t.Run("an unknown policy is a config error at load", func(t *testing.T) {
		_, err := LoadConfig(writeConfigPath(t, groupedYAML(", undeclared_groups: ignore", "[0, 0]")))
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(),
			`field "undeclared_groups" must be drop, error or other, got ignore`) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("the spike's grouped twin, groups declared, reproduces grouped_aggregation exactly", func(t *testing.T) {
		macro, err := RunToStorage(LoadApiRunConfigFromYaml("testdata/grouped/grouped_macro.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		twin, err := RunToStorage(LoadApiRunConfigFromYaml("testdata/grouped/grouped_twin.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "twin vs macro", twin.Storage, macro.Storage)
	})
}
