package api

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMacrosConfigsReuseTheirExpansion: a loaded macros: config expands on its
// first run and reuses that expansion, which still reads its inputs afresh on
// every run; a run that could not expand leaves nothing kept.
func TestMacrosConfigsReuseTheirExpansion(t *testing.T) {
	csv := filepath.Join(t.TempDir(), "x.csv")
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(csv, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	load := func(t *testing.T) *ApiRunConfig {
		t.Helper()
		config, err := LoadConfig(writeConfigPath(t, fmt.Sprintf(`inputs:
  d: {source: {csv: {path: %q, time_column: 0, state_columns: {x: [1]}}}}
macros:
- {type: vector_mean, name: m, data: {partition_name: x}, window: 1}
`, csv)))
		if err != nil {
			t.Fatal(err)
		}
		return config
	}
	means := func(t *testing.T, config *ApiRunConfig) string {
		t.Helper()
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(result.Storage.GetValues("x"))
	}

	t.Run("a second run reuses the expansion and reads its inputs afresh", func(t *testing.T) {
		write("0,1\n1,2\n2,3\n")
		config := load(t)
		if got := means(t, config); got != "[[1] [2] [3]]" {
			t.Fatalf("first run read %s", got)
		}
		first := config.expansion.config
		if first == nil {
			t.Fatal("the first run kept no expansion")
		}
		write("0,10\n1,20\n2,30\n")
		if got := means(t, config); got != "[[10] [20] [30]]" {
			t.Errorf("second run read %s, want the rewritten file's [[10] [20] [30]]", got)
		}
		if config.expansion.config != first {
			t.Error("the second run expanded again instead of reusing the first run's expansion")
		}
	})

	t.Run("a run that could not expand keeps nothing, so the next one tries again", func(t *testing.T) {
		if err := os.Remove(csv); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		config := load(t)
		if _, err := RunToStorage(config); KindOf(err) != ErrUnavailable {
			t.Fatalf("expected the missing input to be unavailable, got %v", err)
		}
		if config.expansion.config != nil {
			t.Fatal("a failed expansion was kept")
		}
		write("0,4\n1,5\n2,6\n")
		if got := means(t, config); got != "[[4] [5] [6]]" {
			t.Errorf("once the file exists the run read %s, want [[4] [5] [6]]", got)
		}
	})
}
