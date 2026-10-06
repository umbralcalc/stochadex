package analysis

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCsvLoaderReturnsErrors pins that a missing or unparseable file comes back
// as an error the caller can handle — it used to log.Fatal, exiting the process.
func TestCsvLoaderReturnsErrors(t *testing.T) {
	t.Run("a missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "absent.csv")
		_, err := NewStateTimeStorageFromCsv(path, 0, map[string][]int{"d": {1}}, false)
		if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), path) {
			t.Errorf("expected an error naming the path and wrapping fs.ErrNotExist, got %v", err)
		}
	})
	t.Run("a file that is not valid CSV", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.csv")
		if err := os.WriteFile(path, []byte("0,\"unterminated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := NewStateTimeStorageFromCsv(path, 0, map[string][]int{"d": {1}}, false)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("expected an error naming the path, got %v", err)
		}
	})
}
