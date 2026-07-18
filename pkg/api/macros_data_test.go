package api

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCsvDataSource writes a CSV, loads it as the data: source, and runs a
// vector_mean macro over it — proving the file-source path (no sub-simulation).
func TestCsvDataSource(t *testing.T) {
	// A CSV of time, x, y where the mean of (x, y) is about (2, -3).
	var b strings.Builder
	for i := 0; i < 400; i++ {
		x := 2.0 + math.Sin(float64(i))
		y := -3.0 + math.Cos(float64(i))
		fmt.Fprintf(&b, "%d,%g,%g\n", i, x, y)
	}
	csvPath := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(csvPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`data:
  source:
    csv:
      path: %q
      time_column: 0
      state_columns: {stream: [1, 2]}
      skip_header: false
macros:
- type: vector_mean
  name: rolling_mean
  data: {partition_name: stream}
  kernel: {type: exponential}
  params: {exponential_weighting_timescale: [50.0]}
  window: 200
`, csvPath)
	out := runMacroConfig(t, cfg)
	rows := out["rolling_mean"]
	if len(rows) == 0 {
		t.Fatal("no rolling_mean output from the CSV source")
	}
	final := rows[len(rows)-1]
	for i, want := range []float64{2.0, -3.0} {
		if math.Abs(final[i]-want) > 0.3 {
			t.Errorf("rolling_mean[%d] = %v, want ~%v", i, final[i], want)
		}
	}
}

func TestDataSourceErrors(t *testing.T) {
	t.Run("empty source", func(t *testing.T) {
		if _, err := (&DataSource{}).load(); err == nil {
			t.Error("expected an error for an empty data source")
		}
	})
	t.Run("more than one source", func(t *testing.T) {
		src := &DataSource{Csv: &csvSource{}, JsonLog: &jsonLogSource{}}
		if _, err := src.load(); err == nil {
			t.Error("expected an error when more than one source is set")
		}
	})
}
