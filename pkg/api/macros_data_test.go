package api

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
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

// TestRegisterDataSource covers the extension mechanism the Arrow and S3 sources are built
// on: data.source is a closed struct, so a source whose dependency the engine cannot carry
// reaches the config surface only through this registry.
func TestRegisterDataSource(t *testing.T) {
	var gotFields map[string]interface{}
	RegisterDataSource("test_source", func(
		fields map[string]interface{},
	) (*simulator.StateTimeStorage, error) {
		gotFields = fields
		storage := simulator.NewStateTimeStorage()
		storage.Append("registered", 0.0, []float64{1.0})
		return storage, nil
	})

	t.Run("a registered source is dispatched, with its fields", func(t *testing.T) {
		source := &DataSource{Extra: map[string]map[string]interface{}{
			"test_source": {"path": "somewhere", "depth": 3},
		}}
		storage, err := source.load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if names := storage.GetNames(); len(names) != 1 || names[0] != "registered" {
			t.Errorf("registered builder's storage not returned, got names %v", names)
		}
		// The builder must receive the spec's fields verbatim — that is how a source gets
		// its own options (a bucket, a key, a format).
		if gotFields["path"] != "somewhere" || gotFields["depth"] != 3 {
			t.Errorf("fields not passed through: %v", gotFields)
		}
	})

	t.Run("an unknown source lists the ones that ARE available", func(t *testing.T) {
		// The discoverability property: a caller naming a source this binary does not
		// carry must learn what it can use, rather than just failing.
		source := &DataSource{Extra: map[string]map[string]interface{}{
			"gcs": {"bucket": "b"},
		}}
		_, err := source.load()
		if err == nil {
			t.Fatal("expected an error for an unknown source")
		}
		for _, expected := range []string{"gcs", "csv", "json_log", "postgres", "test_source"} {
			if !strings.Contains(err.Error(), expected) {
				t.Errorf("error should mention %q, got: %v", expected, err)
			}
		}
	})

	t.Run("registering the same name twice panics", func(t *testing.T) {
		// A silent overwrite would mean whichever module initialised last quietly wins.
		defer func() {
			if recover() == nil {
				t.Error("expected a panic on duplicate registration")
			}
		}()
		RegisterDataSource("test_source", func(
			map[string]interface{},
		) (*simulator.StateTimeStorage, error) {
			return nil, nil
		})
	})
}

// TestDataSourceLoadErrors covers the guards on the source block itself.
func TestDataSourceLoadErrors(t *testing.T) {
	t.Run("setting more than one source is rejected", func(t *testing.T) {
		source := &DataSource{
			JsonLog: &jsonLogSource{Path: "a.log"},
			Extra:   map[string]map[string]interface{}{"whatever": {}},
		}
		_, err := source.load()
		if err == nil || !strings.Contains(err.Error(), "more than one source") {
			t.Errorf("expected a 'pick one' error, got: %v", err)
		}
	})

	t.Run("an empty source is rejected", func(t *testing.T) {
		_, err := (&DataSource{}).load()
		if err == nil || !strings.Contains(err.Error(), "empty") {
			t.Errorf("expected an 'empty source' error, got: %v", err)
		}
	})
}

// TestLoadFormat covers the helper a transport uses after fetching an object: it must reuse
// the local loader for the declared format, including that loader's own field handling, so
// a remote CSV behaves exactly like a local one.
func TestLoadFormat(t *testing.T) {
	t.Run("delegates to the csv loader, honouring its fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "in.csv")
		if err := os.WriteFile(path, []byte("t,v\n0,1.5\n1,2.5\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		storage, err := LoadFormat("csv", map[string]interface{}{
			"path":          path,
			"time_column":   0,
			"state_columns": map[string][]int{"series": {1}},
			"skip_header":   true, // proves the format's own options are applied
		})
		if err != nil {
			t.Fatalf("LoadFormat: %v", err)
		}
		values := storage.GetValues("series")
		if len(values) != 2 {
			t.Fatalf("expected 2 rows (header skipped), got %d", len(values))
		}
		if values[0][0] != 1.5 || values[1][0] != 2.5 {
			t.Errorf("values not loaded correctly: %v", values)
		}
	})

	t.Run("an unknown format is rejected", func(t *testing.T) {
		if _, err := LoadFormat("parquet", map[string]interface{}{"path": "x"}); err == nil {
			t.Fatal("expected an error for an unsupported format")
		}
	})
}
