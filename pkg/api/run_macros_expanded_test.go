package api

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withJSONLog is a config with its outputs written to a json_log at path.
func withJSONLog(t *testing.T, config, path string) string {
	t.Helper()
	if strings.Contains(config, "\noutputs:") {
		t.Fatal("the config already declares outputs")
	}
	return config + fmt.Sprintf("outputs:\n- {name: log, condition: {type: every_step}, "+
		"function: {type: json_log, path: %q}}\n", path)
}

func TestMacrosConfigsRunAsTheirExpansion(t *testing.T) {
	t.Run("the CLI writes what the expanded config writes, byte for byte", func(t *testing.T) {
		dir := t.TempDir()
		macros := withJSONLog(t, readFile(t, "../../cfg/example_posterior_macro_config.yaml"),
			filepath.Join(dir, "macros.log"))
		config, err := LoadConfig(writeConfigPath(t, macros))
		if err != nil {
			t.Fatal(err)
		}
		expanded, err := ExpandToYAML(config)
		if err != nil {
			t.Fatal(err)
		}
		expandedPath := writeConfigPath(t, strings.Replace(string(expanded),
			filepath.Join(dir, "macros.log"), filepath.Join(dir, "expanded.log"), 1))
		for _, path := range []string{writeConfigPath(t, macros), expandedPath} {
			if err := Execute([]string{"stochadex", "--config", path}); err != nil {
				t.Fatal(err)
			}
		}
		written, err := os.ReadFile(filepath.Join(dir, "macros.log"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(dir, "expanded.log"))
		if err != nil {
			t.Fatal(err)
		}
		if len(written) == 0 || !bytes.Equal(written, want) {
			t.Fatalf("the macros config wrote %d bytes, its expansion %d, and they differ",
				len(written), len(want))
		}

	})

	t.Run("a macros config that was not loaded does not run, saying how to load it", func(t *testing.T) {
		config, err := LoadConfig("../../cfg/example_macro_config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		config.expansion = nil // as a config not loaded from text has
		_, err = RunToStorage(config)
		if KindOf(err) != ErrConfig || !strings.Contains(err.Error(), "load it with LoadConfig") {
			t.Errorf("expected ErrConfig saying to load it, got %v", err)
		}
	})

	t.Run("captured views see the run as it outputs", func(t *testing.T) {
		config, err := LoadConfig("../../cfg/example_macro_config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunWith(config, CaptureView("all", nil))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "captured view", result.Views["all"], result.Storage)
		if _, leaked := result.Views["\x00macros"]; leaked {
			t.Error("the run's own every-row capture leaked into the views")
		}
	})
}
