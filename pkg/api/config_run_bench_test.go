package api

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// benchmarkConfig is a run of Wiener processes as a config, with its output
// function and, optionally, an execution strategy. These measure the config
// path end to end — load once, then Run — which is where the output views
// every config now goes through would show up.
func benchmarkConfig(b *testing.B, partitions, width, steps int, output, strategy string) {
	b.Helper()
	var yaml strings.Builder
	yaml.WriteString("main:\n  partitions:\n")
	values := strings.TrimSuffix(strings.Repeat("1.0, ", width), ", ")
	zeros := strings.TrimSuffix(strings.Repeat("0.0, ", width), ", ")
	for i := range partitions {
		fmt.Fprintf(&yaml, "  - {name: w%d, iteration: {type: wiener_process}, params: {variances: [%s]}, "+
			"init_state_values: [%s], state_history_depth: 1, seed: %d}\n", i, values, zeros, i+1)
	}
	fmt.Fprintf(&yaml, "  simulation:\n    output_condition: {type: every_step}\n    output_function: %s\n", output)
	if strategy != "" {
		fmt.Fprintf(&yaml, "    execution_strategy: {type: %s}\n", strategy)
	}
	fmt.Fprintf(&yaml, "    termination_condition: {type: number_of_steps, max_steps: %d}\n"+
		"    timestep_function: {type: constant, stepsize: 1.0}\n    init_time_value: 0.0\n", steps)
	config := writeConfig(b, yaml.String())
	b.ReportAllocs()
	for b.Loop() {
		Run(config, &SocketConfig{})
	}
}

func jsonLogTo(b *testing.B) string {
	return fmt.Sprintf("{type: json_log, path: %q}", filepath.Join(b.TempDir(), "run.log"))
}

func BenchmarkConfigRunTinyInline(b *testing.B) {
	benchmarkConfig(b, 1, 1, 2000, "{type: nil}", "inline")
}

func BenchmarkConfigRunWideInline(b *testing.B) {
	benchmarkConfig(b, 16, 4, 500, "{type: nil}", "inline")
}

func BenchmarkConfigRunWide(b *testing.B) {
	benchmarkConfig(b, 16, 4, 500, "{type: nil}", "")
}

func BenchmarkConfigRunTinyInlineJsonLog(b *testing.B) {
	benchmarkConfig(b, 1, 1, 2000, jsonLogTo(b), "inline")
}

func BenchmarkConfigRunWideJsonLog(b *testing.B) {
	benchmarkConfig(b, 16, 4, 500, jsonLogTo(b), "")
}

// paramsDrivenYAML is a 2000-step inline run whose walk's variances come from a
// CSV input, either through params_from_input or, the older way, through a
// from_input partition read with params_from_upstream (listed first: inline
// execution runs a producer before its consumer).
func paramsDrivenYAML(b *testing.B, fromInput bool) string {
	b.Helper()
	var csv strings.Builder
	for i := range 2001 {
		fmt.Fprintf(&csv, "%d,%v\n", i, 1+float64(i%7))
	}
	path := filepath.Join(b.TempDir(), "vol.csv")
	writeFile(b, path, csv.String())
	binding := "params_from_input: {variances: {input: vol}}"
	extra := ""
	if !fromInput {
		binding = "params_from_upstream: {variances: {upstream: variances}}"
		extra = "  - {name: variances, iteration: {type: from_input, input: vol}, state_history_depth: 1, seed: 0}\n"
	}
	return fmt.Sprintf(`inputs:
  vol: {source: {csv: {path: %q, time_column: 0, state_columns: {variances: [1]}}}}
main:
  partitions:
%s  - {name: walk, iteration: {type: wiener_process}, params: {variances: [1.0]}, %s, init_state_values: [0.0], state_history_depth: 1, seed: 3}
  simulation:
    output_condition: {type: every_step}
    output_function: {type: nil}
    execution_strategy: {type: inline}
    termination_condition: {type: number_of_steps, max_steps: 2000}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`, path, extra, binding)
}

func benchmarkParamsDriven(b *testing.B, fromInput bool) {
	config := writeConfig(b, paramsDrivenYAML(b, fromInput))
	b.ReportAllocs()
	for b.Loop() {
		Run(config, &SocketConfig{})
	}
}

// BenchmarkConfigRunParamsFromInput sets a param from an input between steps.
func BenchmarkConfigRunParamsFromInput(b *testing.B) { benchmarkParamsDriven(b, true) }

// BenchmarkConfigRunParamsFromUpstream does the same through a from_input
// partition and params_from_upstream, for comparison.
func BenchmarkConfigRunParamsFromUpstream(b *testing.B) { benchmarkParamsDriven(b, false) }
