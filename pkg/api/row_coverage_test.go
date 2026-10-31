package api

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// Every row of a macro's data counts, its first row included: an input's first
// row is its replay's initial state, which the run never steps to, so a macro
// that only folds in each step's latest value misses it. This suite runs each
// macro that aggregates its data's rows over one CSV whose first row stands
// out, in both tiers, and compares it with the same aggregate computed here
// from the rows themselves.

// coverageRows are the CSV's rows after its time column: a group (0 or 1), a
// count and an exposure. The first row's values appear nowhere else.
var coverageRows = [][]float64{
	{0, 7, 20}, {1, 0, 10}, {0, 1, 10}, {1, 3, 10}, {0, 1, 12}, {1, 2, 10},
}

// coverageSteps is how many steps a replay of coverageRows runs for.
var coverageSteps = len(coverageRows) - 1

// coverageCase is a macro over the CSV and the aggregate it must give.
type coverageCase struct {
	// macro is the macros: entry, naming its partition "out".
	macro string
	// want is the macro's state after the given step (0 is the initial
	// state), from the rows up to it, or nil where the macro defines none
	// (a windowed aggregation before its window has filled).
	want func(step int) []float64
}

// columnOf is a column of the rows up to and including row last.
func columnOf(column, last int) []float64 {
	values := make([]float64, last+1)
	for i := range values {
		values[i] = coverageRows[i][column]
	}
	return values
}

func mean(values []float64) float64 { return floats.Sum(values) / float64(len(values)) }

// groupedBy aggregates the count column of the rows up to last within each
// group, in group order.
func groupedBy(last int, aggregate func([]float64) float64) []float64 {
	var groups [2][]float64
	for _, row := range coverageRows[:last+1] {
		groups[int(row[0])] = append(groups[int(row[0])], row[1])
	}
	return []float64{aggregate(groups[0]), aggregate(groups[1])}
}

// windowed is a windowed aggregation's want: its value over every row from the
// step its window of coverageSteps rows fills, and none before.
func windowed(final func() []float64) func(int) []float64 {
	return func(step int) []float64 {
		if step < coverageSteps {
			return nil
		}
		return final()
	}
}

// afterFirstStep is a want from step 1 on: before it, a grouped aggregation's
// state is its default_values.
func afterFirstStep(want func(int) []float64) func(int) []float64 {
	return func(step int) []float64 {
		if step == 0 {
			return nil
		}
		return want(step)
	}
}

const coverageWindow = ", window: 5}\n" // coverageSteps

var coverageCases = map[string]coverageCase{
	"vector_mean cumulative": {
		macro: "- {type: vector_mean, name: out, data: {partition_name: rows, value_indices: [1, 2]}, cumulative: true}\n",
		want: func(step int) []float64 {
			return []float64{mean(columnOf(1, step)), mean(columnOf(2, step))}
		},
	},
	"vector_mean": {
		macro: "- {type: vector_mean, name: out, data: {partition_name: rows, value_indices: [1, 2]}, " +
			"kernel: {type: constant}" + coverageWindow,
		want: windowed(func() []float64 {
			return []float64{mean(columnOf(1, coverageSteps)), mean(columnOf(2, coverageSteps))}
		}),
	},
	"vector_variance": {
		macro: "- {type: vector_mean, name: m, data: {partition_name: rows, value_indices: [1]}, cumulative: true}\n" +
			"- {type: vector_variance, name: out, data: {partition_name: rows, value_indices: [1]}, " +
			"mean: {partition_name: m}, kernel: {type: constant}" + coverageWindow,
		want: windowed(func() []float64 {
			counts := columnOf(1, coverageSteps)
			m, squares := mean(counts), 0.0
			for _, count := range counts {
				squares += (count - m) * (count - m)
			}
			return []float64{squares / float64(len(counts)-1)}
		}),
	},
	"vector_covariance": {
		macro: "- {type: vector_mean, name: m, data: {partition_name: rows, value_indices: [1, 2]}, cumulative: true}\n" +
			"- {type: vector_covariance, name: out, data: {partition_name: rows, value_indices: [1, 2]}, " +
			"mean: {partition_name: m}, kernel: {type: constant}" + coverageWindow,
		want: windowed(func() []float64 {
			counts, exposures := columnOf(1, coverageSteps), columnOf(2, coverageSteps)
			mc, me := mean(counts), mean(exposures)
			var cc, ce, ee float64
			for i := range counts {
				cc += (counts[i] - mc) * (counts[i] - mc)
				ce += (counts[i] - mc) * (exposures[i] - me)
				ee += (exposures[i] - me) * (exposures[i] - me)
			}
			// The sample covariance's upper triangle, row-major, as the
			// iteration returns it.
			n := float64(len(counts) - 1)
			return []float64{cc / n, ce / n, 0, ee / n}
		}),
	},
	"grouped_aggregation count": {
		macro: "- {type: grouped_aggregation, name: out, aggregation: count, precision: 0, " +
			"data: {partition_name: rows, value_indices: [1]}, " +
			"group_by: [{partition_name: rows, value_indices: [0]}], groups: [[0], [1]], " +
			"kernel: {type: constant}" + coverageWindow,
		want: afterFirstStep(func(step int) []float64 {
			return groupedBy(step, func(values []float64) float64 { return float64(len(values)) })
		}),
	},
	"grouped_aggregation sum": {
		macro: "- {type: grouped_aggregation, name: out, aggregation: sum, precision: 0, " +
			"data: {partition_name: rows, value_indices: [1]}, " +
			"group_by: [{partition_name: rows, value_indices: [0]}], groups: [[0], [1]], " +
			"kernel: {type: constant}" + coverageWindow,
		want: afterFirstStep(func(step int) []float64 { return groupedBy(step, floats.Sum) }),
	},
	"scalar_regression_stats cumulative": {
		macro: "- {type: scalar_regression_stats, name: out, y: {partition_name: rows, value_indices: [1]}, " +
			"x: {partition_name: rows, value_indices: [2]}, intercept: true}\n",
		want: func(step int) []float64 {
			// The sums the estimates are made from: [n, Sx, Sy, Sxx, Sxy, Syy].
			counts, exposures := columnOf(1, step), columnOf(2, step)
			return []float64{float64(step + 1), floats.Sum(exposures), floats.Sum(counts),
				floats.Dot(exposures, exposures), floats.Dot(exposures, counts), floats.Dot(counts, counts)}
		},
	},
	"scalar_regression_stats window": {
		// A window as long as the data: it keeps every pair seen.
		macro: "- {type: scalar_regression_stats, name: out, y: {partition_name: rows, value_indices: [1]}, " +
			"x: {partition_name: rows, value_indices: [2]}, intercept: true, mode: window, window_length: 6}\n",
		want: func(step int) []float64 {
			counts, exposures := columnOf(1, step), columnOf(2, step)
			return []float64{float64(step + 1), floats.Sum(exposures), floats.Sum(counts),
				floats.Dot(exposures, exposures), floats.Dot(exposures, counts), floats.Dot(counts, counts)}
		},
	},
}

// coverageExcluded are the macros the suite leaves out, each with why.
var coverageExcluded = map[string]string{
	"likelihood_comparison":           "a model's likelihood over the data window, which it reads as vector_mean does; its score has no closed form to compare with here",
	"posterior_estimation":            "a posterior updated from windowed likelihoods, read as likelihood_comparison does; no closed form",
	"likelihood_mean_function_fit":    "a gradient fit over the data window; no closed form",
	"evolution_strategy_optimisation": "runs on its own clock and reads no input rows",
	"smc_inference":                   "reads the inputs only inside its own particle runs, each from the input's first row",
	"mcts_self_play":                  "searches a registered environment and reads no input rows",
	"mcts_planning":                   "searches a forward model on its own clock and reads no input rows",
}

// coverageState is the part of a macro's state its case checks: all of it, or
// for scalar_regression_stats its sums.
func coverageState(name string, state []float64) []float64 {
	switch name {
	case "scalar_regression_stats cumulative":
		return state[:6]
	case "scalar_regression_stats window":
		// Past the ring of 6 pairs and its count.
		return state[13:19]
	}
	return state
}

// coverageConfig loads the CSV with a case's macros.
func coverageConfig(t *testing.T, macro string) func() *ApiRunConfig {
	dir := t.TempDir()
	var csv strings.Builder
	csv.WriteString("time,group,count,exposure\n")
	for i, row := range coverageRows {
		fmt.Fprintf(&csv, "%d,%v,%v,%v\n", i+1, row[0], row[1], row[2])
	}
	writeFile(t, filepath.Join(dir, "rows.csv"), csv.String())
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, `inputs:
  data:
    source:
      csv:
        path: `+filepath.Join(dir, "rows.csv")+`
        time_column: 0
        state_columns: {rows: [1, 2, 3]}
        skip_header: true
macros:
`+macro)
	return func() *ApiRunConfig {
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		return config
	}
}

func TestMacrosCountEveryRowOnce(t *testing.T) {
	t.Run("every registered macro is covered or excluded with a reason", func(t *testing.T) {
		covered := map[string]bool{}
		for name := range coverageCases {
			covered[strings.Fields(name)[0]] = true
		}
		var missing []string
		for macro := range macroSpecFactories {
			if !covered[macro] && coverageExcluded[macro] == "" {
				missing = append(missing, macro)
			}
			if covered[macro] && coverageExcluded[macro] != "" {
				t.Errorf("%s is both covered and excluded", macro)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("macros neither in coverageCases nor in coverageExcluded: %v", missing)
		}
		for macro := range coverageExcluded {
			if macroSpecFactories[macro] == nil {
				t.Errorf("coverageExcluded names %q, which is not a registered macro", macro)
			}
		}
	})

	names := make([]string, 0, len(coverageCases))
	for name := range coverageCases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := coverageCases[name]
		t.Run(name, func(t *testing.T) {
			for tier, storage := range bothTiers(t, coverageConfig(t, c.macro)) {
				checkCoverage(t, tier, name, storage, c.want)
			}
		})
	}
}

// checkCoverage compares a macro's every recorded step with its case's want.
func checkCoverage(t *testing.T, tier, name string, storage *simulator.StateTimeStorage,
	want func(int) []float64) {
	t.Helper()
	rows := storage.GetValues("out")
	if len(rows) != len(coverageRows) {
		t.Fatalf("%s: %d rows of out, want %d", tier, len(rows), len(coverageRows))
	}
	defined := 0
	for step, row := range rows {
		expected := want(step)
		if expected == nil {
			continue
		}
		defined++
		if got := coverageState(name, row); !floats.EqualApprox(got, expected, 1e-12) {
			t.Errorf("%s: after step %d, out = %v, want %v from rows 0..%d", tier, step, got, expected, step)
		}
	}
	if defined == 0 {
		t.Fatalf("%s: the case defines no step to check", tier)
	}
}
