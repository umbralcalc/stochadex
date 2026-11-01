package api

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// csvRows are the counts and exposures a CSV input below holds, one row per
// time. Its first row is distinctive, so a macro that missed it would show.
var csvRows = [][]float64{{7, 20}, {0, 10}, {1, 10}, {3, 10}, {1, 10}}

// csvMacroConfig writes csvRows to a CSV beside a config with the given
// macros: and outputs:, and loads it.
func csvMacroConfig(t *testing.T, macros, outputs string) *ApiRunConfig {
	t.Helper()
	dir := t.TempDir()
	var csv strings.Builder
	csv.WriteString("time,count,exposure\n")
	for i, row := range csvRows {
		csv.WriteString(strings.Join([]string{
			ftoa(float64(i + 1)), ftoa(row[0]), ftoa(row[1])}, ",") + "\n")
	}
	writeFile(t, filepath.Join(dir, "rows.csv"), csv.String())
	config := `inputs:
  data:
    source:
      csv:
        path: ` + filepath.Join(dir, "rows.csv") + `
        time_column: 0
        state_columns: {count: [1], exposure: [2], both: [1, 2]}
        skip_header: true
macros:
` + macros + outputs
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, config)
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func ftoa(value float64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// runAsExpansion runs a config as its expansion, failing on an error.
func runAsExpansion(t *testing.T, config func() *ApiRunConfig) *simulator.StateTimeStorage {
	t.Helper()
	result, err := RunToStorage(config())
	if err != nil {
		t.Fatalf("run as its expansion: %v", err)
	}
	return result.Storage
}

func lastRow(storage *simulator.StateTimeStorage, name string) []float64 {
	rows := storage.GetValues(name)
	return rows[len(rows)-1]
}

func TestMacrosCountEveryCSVRow(t *testing.T) {
	var sums [2]float64
	for _, row := range csvRows {
		sums[0] += row[0]
		sums[1] += row[1]
	}
	n := float64(len(csvRows))

	t.Run("a cumulative vector_mean is the mean of every row", func(t *testing.T) {
		config := func() *ApiRunConfig {
			return csvMacroConfig(t, `- type: vector_mean
  name: mean
  data: {partition_name: both}
  cumulative: true
`, "")
		}
		want := []float64{sums[0] / n, sums[1] / n}
		storage := runAsExpansion(t, config)
		if got := lastRow(storage, "mean"); !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("final mean %v, want %v, the mean of all %d rows", got, want, len(csvRows))
		}
		rows := storage.GetValues("mean")
		if !floats.Equal(rows[0], csvRows[0]) {
			t.Errorf("initial mean %v, want the first row %v", rows[0], csvRows[0])
		}
	})

	t.Run("scalar_regression_stats counts the first row", func(t *testing.T) {
		config := func() *ApiRunConfig {
			return csvMacroConfig(t, `- type: scalar_regression_stats
  name: ols
  y: {partition_name: count}
  x: {partition_name: exposure}
  mode: cumulative
`, "")
		}
		var sxy float64
		for _, row := range csvRows {
			sxy += row[0] * row[1]
		}
		storage := runAsExpansion(t, config)
		// Through-origin layout: [Sxx, Sxy, Syy, n, beta, sigma2].
		got := lastRow(storage, "ols")
		if got[3] != n || got[1] != sxy {
			t.Errorf("n = %v, Sxy = %v; want %v and %v, every row counted", got[3], got[1], n, sxy)
		}
	})

	t.Run("a windowed vector_mean longer than the input fails, naming the fix", func(t *testing.T) {
		config := csvMacroConfig(t, `- type: vector_mean
  name: mean
  data: {partition_name: both}
  kernel: {type: constant}
  window: 10
`, "")
		_, err := RunToStorage(config)
		if err == nil || !strings.Contains(err.Error(), "cumulative: true") ||
			!strings.Contains(err.Error(), "window: 10") {
			t.Fatalf("run as its expansion: got %v, want an error naming window: 10 and cumulative: true", err)
		}
	})

	t.Run("a window the input fills still runs", func(t *testing.T) {
		config := func() *ApiRunConfig {
			return csvMacroConfig(t, `- type: vector_mean
  name: mean
  data: {partition_name: both}
  kernel: {type: constant}
  window: 4
`, "")
		}
		want := []float64{sums[0] / n, sums[1] / n}
		storage := runAsExpansion(t, config)
		if got := lastRow(storage, "mean"); !floats.EqualApprox(got, want, 1e-12) {
			t.Errorf("final mean %v, want %v", got, want)
		}
	})

	t.Run("cumulative: true rejects the windowed mean's fields", func(t *testing.T) {
		for field, line := range map[string]string{
			"kernel:":        "  kernel: {type: constant}\n",
			"window:":        "  window: 3\n",
			"params:":        "  params: {exponential_weighting_timescale: [10.0]}\n",
			"default_value:": "  default_value: 1.0\n",
		} {
			macro := "- type: vector_mean\n  name: mean\n  data: {partition_name: both}\n" +
				"  cumulative: true\n" + line
			if _, err := RunToStorage(csvMacroConfig(t, macro, "")); err == nil || !strings.Contains(err.Error(), field) {
				t.Errorf("run as its expansion, with %s: got %v, want an error naming it", field, err)
			}
		}
	})

	t.Run("a cumulative vector_mean of a partition nothing supplies fails, naming it", func(t *testing.T) {
		config := csvMacroConfig(t, "- {type: vector_mean, name: mean, data: {partition_name: nope}, cumulative: true}\n", "")
		if _, err := RunToStorage(config); err == nil || !strings.Contains(err.Error(), `"nope"`) {
			t.Fatalf("got %v, want an error naming the partition nope", err)
		}
	})

	t.Run("a variance or covariance window longer than the input fails", func(t *testing.T) {
		for _, macro := range []string{"vector_variance", "vector_covariance"} {
			config := func() *ApiRunConfig {
				return csvMacroConfig(t, "- {type: vector_mean, name: m, data: {partition_name: both}, cumulative: true}\n"+
					"- {type: "+macro+", name: spread, data: {partition_name: both}, mean: {partition_name: m}, "+
					"kernel: {type: constant}, window: 10}\n", "")
			}
			want := macro + ` "spread" has window: 10, but the run replays its input for only 4 steps`
			if _, err := RunToStorage(config()); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("run as its expansion: got %v, want %q", err, want)
			}
			// Only vector_mean has a cumulative form to point to.
			if _, err := RunToStorage(config()); err != nil && strings.Contains(err.Error(), "cumulative: true") {
				t.Errorf("%s's error points to cumulative: true, which it does not take: %v", macro, err)
			}
		}
	})

	t.Run("a final_step output writes one row per partition", func(t *testing.T) {
		dir := t.TempDir()
		log := filepath.Join(dir, "final.jsonl")
		config := csvMacroConfig(t, `- type: vector_mean
  name: mean
  data: {partition_name: both}
  cumulative: true
`, `outputs:
- {name: final, condition: {type: final_step, partitions: [mean]}, function: {type: json_log, path: `+log+`}}
`)
		if _, err := RunWith(config, WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(log)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		type logEntry struct {
			Partition string    `json:"partition_name"`
			State     []float64 `json:"state"`
			Time      float64   `json:"time"`
		}
		var entries []logEntry
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var entry logEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			entries = append(entries, entry)
		}
		want := []float64{sums[0] / n, sums[1] / n}
		if len(entries) != 1 || entries[0].Partition != "mean" || entries[0].Time != n ||
			!floats.EqualApprox(entries[0].State, want, 1e-12) {
			t.Fatalf("final_step wrote %+v, want one entry: mean %v at time %v", entries, want, n)
		}
	})
}
