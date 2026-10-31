package api

import (
	"path/filepath"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// runningMeanConfig replays csvRows' "both" partition and keeps its running
// mean in an expression partition, with the given init_state_from_upstream
// line (or none).
func runningMeanConfig(t *testing.T, fromUpstream string) string {
	t.Helper()
	dir := t.TempDir()
	var csv strings.Builder
	csv.WriteString("time,count,exposure\n")
	for i, row := range csvRows {
		csv.WriteString(ftoa(float64(i+1)) + "," + ftoa(row[0]) + "," + ftoa(row[1]) + "\n")
	}
	writeFile(t, filepath.Join(dir, "rows.csv"), csv.String())
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, `inputs:
  data:
    source:
      csv: {path: `+filepath.Join(dir, "rows.csv")+`, time_column: 0, state_columns: {both: [1, 2]}, skip_header: true}
main:
  partitions:
  - name: both
    iteration: {type: from_input, input: data}
    state_history_depth: 1
    seed: 0
  - name: mean
    iteration:
      type: expression
      fields: [{name: c}, {name: e}]
      outputs: ["c + (v[0] - c) / (step + 1)", "e + (v[1] - e) / (step + 1)"]
    params: {v: [0.0, 0.0]}
    params_from_upstream: {v: {upstream: both}}
    init_state_values: [0.0, 0.0]
`+fromUpstream+`    state_history_depth: 1
    seed: 0
  simulation:
    timestep_function: {type: from_input, input: data}
    termination_condition: {type: input_exhausted, input: data}
`)
	return path
}

func TestInitStateFromUpstreamConfig(t *testing.T) {
	var sums [2]float64
	for _, row := range csvRows {
		sums[0] += row[0]
		sums[1] += row[1]
	}
	n := float64(len(csvRows))

	t.Run("an expression's running mean started from an input's first row counts every row", func(t *testing.T) {
		config, err := LoadConfig(runningMeanConfig(t, "    init_state_from_upstream: v\n"))
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		means := result.Storage.GetValues("mean")
		if !floats.Equal(means[0], csvRows[0]) {
			t.Errorf("initial state %v, want the input's first row %v", means[0], csvRows[0])
		}
		if got, want := means[len(means)-1], []float64{sums[0] / n, sums[1] / n}; !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("final mean %v, want %v over all %d rows", got, want, len(csvRows))
		}
	})

	t.Run("--check passes it without reading the input", func(t *testing.T) {
		config := runningMeanConfig(t, "    init_state_from_upstream: v\n")
		if err, _ := executeIn(t, t.TempDir(), "--config", config, "--check"); err != nil {
			t.Fatalf("--check: %v", err)
		}
	})

	t.Run("a key params_from_upstream lacks fails --check as the run fails", func(t *testing.T) {
		config := runningMeanConfig(t, "    init_state_from_upstream: w\n")
		checked, _ := executeIn(t, t.TempDir(), "--config", config, "--check")
		ran, _ := executeIn(t, t.TempDir(), "--config", config)
		const want = `init_state_from_upstream "w" names no params_from_upstream key`
		if KindOf(checked) != ErrConfig || !strings.Contains(checked.Error(), want) {
			t.Fatalf("--check: got %v, want a config error containing %q", checked, want)
		}
		if ran == nil || ran.Error() != checked.Error() {
			t.Errorf("--check said %q but the run said %v", checked, ran)
		}
	})
}
