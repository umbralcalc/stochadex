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
