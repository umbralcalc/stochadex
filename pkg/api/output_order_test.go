package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// orderedOutputPartitions are the partitions of the output-order config, in
// config order. The names are deliberately not sorted, so a sink that sorted
// rows by name would fail as surely as one that wrote them as they finished.
var orderedOutputPartitions = []string{"p5", "p2", "p7", "p0", "p6", "p3", "p1", "p4"}

// orderedOutputConfig is a config whose partitions all write every step to one
// sink, under the given execution strategy ("" for the default).
func orderedOutputConfig(function, strategy string) string {
	var config strings.Builder
	config.WriteString("main:\n  partitions:\n")
	for i, name := range orderedOutputPartitions {
		fmt.Fprintf(&config, `  - name: %s
    iteration: {type: wiener_process}
    params: {variances: [1.0, 1.0]}
    init_state_values: [0.0, 0.0]
    state_history_depth: 1
    seed: %d
`, name, 100+i)
	}
	config.WriteString(`  simulation:
    termination_condition: {type: number_of_steps, max_steps: 200}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
`)
	if strategy != "" {
		fmt.Fprintf(&config, "    execution_strategy: {type: %s}\n", strategy)
	}
	fmt.Fprintf(&config, "outputs:\n- {name: out, condition: {type: every_step}, function: %s}\n",
		function)
	return config.String()
}

// TestOutputRowOrderIsDeterministic runs one config twice through the CLI and
// requires byte-identical output, with each step's rows in partition (config)
// order. Partitions work concurrently, so before outputs were ordered two
// identical runs wrote the same rows in a different order within a step.
func TestOutputRowOrderIsDeterministic(t *testing.T) {
	for _, strategy := range []string{"", "spawn_per_step", "persistent_worker", "inline"} {
		label := strategy
		if label == "" {
			label = "default"
		}
		t.Run("json_log, "+label, func(t *testing.T) {
			dir := t.TempDir()
			run := func(name string) []byte {
				path := filepath.Join(dir, name+".log")
				config := filepath.Join(dir, name+".yaml")
				writeFile(t, config, orderedOutputConfig(
					fmt.Sprintf("{type: json_log, path: %q}", path), strategy))
				captureStderr(t, func() {
					if err := Execute([]string{"stochadex", "--config", config}); err != nil {
						t.Fatal(err)
					}
				})
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			first, second := run("first"), run("second")
			if !bytes.Equal(first, second) {
				t.Fatalf("two runs of one config wrote different json_logs:\n%s",
					firstDifference(first, second))
			}
			var names []string
			scanner := bufio.NewScanner(bytes.NewReader(first))
			for scanner.Scan() {
				var entry struct {
					PartitionName string `json:"partition_name"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
					t.Fatal(err)
				}
				names = append(names, entry.PartitionName)
			}
			requireConfigOrder(t, names, 201)
		})
		t.Run("stdout, "+label, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, config, orderedOutputConfig("{type: stdout}", strategy))
			run := func() string {
				var out string
				captureStderr(t, func() {
					out = captureStdout(t, func() {
						if err := Execute([]string{"stochadex", "--config", config}); err != nil {
							t.Error(err)
						}
					})
				})
				return out
			}
			first, second := run(), run()
			if first != second {
				t.Fatalf("two runs of one config printed different output:\n%s",
					firstDifference([]byte(first), []byte(second)))
			}
			var names []string
			for _, line := range strings.Split(strings.TrimSpace(first), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					t.Fatalf("unexpected stdout line %q", line)
				}
				names = append(names, fields[1])
			}
			requireConfigOrder(t, names, 201)
		})
	}
}

// requireConfigOrder requires names, a sink's rows' partitions in the order
// written, to be steps rows each in config order.
func requireConfigOrder(t *testing.T, names []string, steps int) {
	t.Helper()
	width := len(orderedOutputPartitions)
	if len(names) != steps*width {
		t.Fatalf("got %d rows, want %d", len(names), steps*width)
	}
	for i, name := range names {
		if want := orderedOutputPartitions[i%width]; name != want {
			t.Fatalf("row %d (step %d) is partition %s, want %s: rows within a step "+
				"must come out in config order", i, i/width, name, want)
		}
	}
}

// firstDifference describes the first line at which two outputs differ.
func firstDifference(a, b []byte) string {
	linesA, linesB := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := range min(len(linesA), len(linesB)) {
		if linesA[i] != linesB[i] {
			return fmt.Sprintf("line %d:\n  %s\n  %s", i+1, linesA[i], linesB[i])
		}
	}
	return fmt.Sprintf("lengths differ: %d and %d lines", len(linesA), len(linesB))
}
