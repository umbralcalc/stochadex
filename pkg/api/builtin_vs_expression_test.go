package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// builtinPair is one process written twice: as a built-in iteration and as an
// expression doing the same arithmetic with the same draws. Each config's
// partition named out is the process; the partitions before it are shared
// sources, written identically in both.
type builtinPair struct {
	name string
	// sources are the partitions out reads, for a width.
	sources func(width int) string
	// builtin and expression are out, for a width.
	builtin, expression func(width int) string
	// warmUp is how many steps out takes to mean the same in both: a
	// windowed mean returns its default until its window has filled.
	warmUp int
}

// partition writes one partition of the given width.
func partition(name, iteration, params, wiring string, initial float64, width, depth int) string {
	line := fmt.Sprintf("  - name: %s\n    iteration: %s\n", name, iteration)
	if params != "" {
		line += "    params: {" + params + "}\n"
	}
	line += wiring
	return line + fmt.Sprintf("    init_state_values: %s\n    state_history_depth: %d\n    seed: 7\n",
		repeatedList(initial, width), depth)
}

// repeatedList is a YAML list of width copies of value.
func repeatedList(value float64, width int) string {
	items := make([]string, width)
	for i := range items {
		items[i] = fmt.Sprint(value)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

// expressionIteration is an expression iteration of one field of width
// width, with the given update.
func expressionIteration(width int, upstreams, update string) string {
	return fmt.Sprintf("{type: expression, fields: [{name: x, width: %d}]%s, outputs: [%q]}",
		width, upstreams, update)
}

// builtinPairs are the processes compared.
var builtinPairs = []builtinPair{
	{
		name: "wiener_process",
		builtin: func(w int) string {
			return partition("out", "{type: wiener_process}", "variances: "+repeatedList(2, w), "", 0, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, "", fmt.Sprintf(
				"x + sqrt(variances * dt) * iid(%d, normal(0, 1))", w)),
				"variances: "+repeatedList(2, w), "", 0, w, 1)
		},
	},
	{
		name: "ornstein_uhlenbeck",
		builtin: func(w int) string {
			return partition("out", "{type: ornstein_uhlenbeck}", fmt.Sprintf("thetas: %s, mus: %s, sigmas: %s",
				repeatedList(1.5, w), repeatedList(2, w), repeatedList(0.5, w)), "", 0, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, "",
				fmt.Sprintf("x + thetas * (mus - x) * dt + sigmas * sqrt(dt) * iid(%d, normal(0, 1))", w)),
				fmt.Sprintf("thetas: %s, mus: %s, sigmas: %s",
					repeatedList(1.5, w), repeatedList(2, w), repeatedList(0.5, w)), "", 0, w, 1)
		},
	},
	{
		name: "geometric_brownian_motion",
		builtin: func(w int) string {
			return partition("out", "{type: geometric_brownian_motion}", "variances: "+repeatedList(0.01, w), "", 1, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, "", fmt.Sprintf(
				"x * (1 + sqrt(variances * dt) * iid(%d, normal(0, 1)))", w)),
				"variances: "+repeatedList(0.01, w), "", 1, w, 1)
		},
	},
	{
		name: "poisson_process",
		builtin: func(w int) string {
			return partition("out", "{type: poisson_process}", "rates: "+repeatedList(3, w), "", 0, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, "", fmt.Sprintf(
				"x + where(iid(%d, uniform(0, 1)) * (rates + 1 / dt) < rates, 1, 0)", w)),
				"rates: "+repeatedList(3, w), "", 0, w, 1)
		},
	},
	{
		// A Cox process as building blocks, a time-varying rate feeding a plain
		// Poisson process, against one expression doing both.
		name: "cox_from_building_blocks",
		sources: func(w int) string {
			return partition("rate", expressionIteration(w, "", "2 + sin(t)"), "", "", 2, w, 1)
		},
		builtin: func(w int) string {
			return partition("out", "{type: poisson_process}", "rates: "+repeatedList(0, w),
				"    params_from_upstream: {rates: {upstream: rate}}\n", 0, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, "", fmt.Sprintf(
				"x + where(iid(%d, uniform(0, 1)) * (2 + sin(t) + 1 / dt) < 2 + sin(t), 1, 0)", w)),
				"", "", 0, w, 1)
		},
	},
	{
		name: "cumulative_mean",
		sources: func(w int) string {
			return partition("src", expressionIteration(w, "", fmt.Sprintf("sin(0.3 * t + each(%d, i, i))", w)),
				"", "", 0, w, 1)
		},
		builtin: func(w int) string {
			return partition("out", "{type: values_function_cumulative_mean, function: data_values}", "",
				"    params_as_partitions: {data_values_partition: [src]}\n"+
					"    params_from_upstream: {latest_data_values: {upstream: src}}\n"+
					"    init_state_from_upstream: latest_data_values\n", 0, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, "", "x + (v - x) / (step + 1)"),
				"v: "+repeatedList(0, w),
				"    params_from_upstream: {v: {upstream: src}}\n    init_state_from_upstream: v\n", 0, w, 1)
		},
	},
	{
		// A mean over this step's value and the 19 before it.
		name: "rolling_window_mean",
		sources: func(w int) string {
			return partition("src", expressionIteration(w, "", fmt.Sprintf("sin(0.3 * t + each(%d, i, i))", w)),
				"", "", 0, w, 19)
		},
		builtin: func(w int) string {
			return partition("out",
				"{type: values_function_vector_mean, function: data_values, kernel: {type: constant}}", "",
				"    params_as_partitions: {data_values_partition: [src]}\n"+
					"    params_from_upstream: {latest_data_values: {upstream: src}}\n", 0, w, 1)
		},
		expression: func(w int) string {
			return partition("out", expressionIteration(w, ", upstreams: {h: src}",
				"(v + "+lagSum("h", 19)+") / 20"),
				"v: "+repeatedList(0, w), "    params_from_upstream: {v: {upstream: src}}\n", 0, w, 1)
		},
		warmUp: 19,
	},
}

// lagSum is the sum of an upstream's last n rows, row by row: lag(h, 0) + ...
func lagSum(alias string, n int) string {
	terms := make([]string, n)
	for i := range terms {
		terms[i] = fmt.Sprintf("lag(%s, %d)", alias, i)
	}
	return strings.Join(terms, " + ")
}

// pairConfig is a config of one side of a pair, for steps of 0.1. Without
// record, it outputs nothing, so a run times only its arithmetic.
func pairConfig(pair builtinPair, out func(int) string, width, steps int, record bool) string {
	config := "main:\n  partitions:\n"
	if pair.sources != nil {
		config += pair.sources(width)
	}
	config += out(width) + fmt.Sprintf(`  simulation:
    termination_condition: {type: number_of_steps, max_steps: %d}
    timestep_function: {type: constant, stepsize: 0.1}
    init_time_value: 0
`, steps)
	if !record {
		config += "outputs:\n- {name: none, condition: {type: nil}, function: {type: nil}}\n"
	}
	return config
}

func TestBuiltinsAndExpressionsCompute(t *testing.T) {
	for _, pair := range builtinPairs {
		for _, width := range []int{1, 8} {
			t.Run(fmt.Sprintf("%s/width=%d", pair.name, width), func(t *testing.T) {
				builtin := runToStorageT(t, pairConfig(pair, pair.builtin, width, 300, true))
				expression := runToStorageT(t, pairConfig(pair, pair.expression, width, 300, true))
				got, want := expression.GetValues("out"), builtin.GetValues("out")
				if len(got) != 301 || len(want) != 301 {
					t.Fatalf("got %d and %d rows, want 301", len(got), len(want))
				}
				moved := false
				for step := pair.warmUp; step < len(want); step++ {
					// The same arithmetic, in a different order: equal to rounding.
					if !floats.EqualApprox(got[step], want[step], 1e-9) {
						t.Fatalf("step %d: the expression gives %v, the built-in %v", step, got[step], want[step])
					}
					moved = moved || !floats.Equal(want[step], want[0])
				}
				if !moved {
					t.Fatal("the process never moved, so the comparison shows nothing")
				}
			})
		}
	}
}

// runToStorageT runs a config, failing the test on an error.
func runToStorageT(t *testing.T, yaml string) *simulator.StateTimeStorage {
	t.Helper()
	result, err := RunToStorage(writeConfig(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	return result.Storage
}

// BenchmarkBuiltinVsExpression runs each process for 1000 steps of width 1
// and 100, as a built-in and as the expression doing the same arithmetic,
// outputting nothing. Results are recorded in docs/performance.md.
func BenchmarkBuiltinVsExpression(b *testing.B) {
	for _, pair := range builtinPairs {
		for _, width := range []int{1, 100} {
			for _, side := range []struct {
				name string
				out  func(int) string
			}{{"builtin", pair.builtin}, {"expression", pair.expression}} {
				b.Run(fmt.Sprintf("%s/width=%d/%s", pair.name, width, side.name), func(b *testing.B) {
					config := writeConfig(b, pairConfig(pair, side.out, width, 1000, false))
					b.ReportAllocs()
					for b.Loop() {
						if _, err := RunToStorage(config); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
