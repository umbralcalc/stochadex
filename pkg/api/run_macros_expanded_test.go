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

		// Macro by macro, the result is replayed through the outputs in name
		// order within each row; one run writes each step in partition order.
		// The two hold the same lines, so the switch shows in their order.
		replayed := filepath.Join(dir, "replayed.log")
		reference, err := LoadConfig(writeConfigPath(t, strings.Replace(macros,
			filepath.Join(dir, "macros.log"), replayed, 1)))
		if err != nil {
			t.Fatal(err)
		}
		reference.source = nil // assembled in Go: runs macro by macro
		if _, err := RunWith(reference, WithConfigOutputs()); err != nil {
			t.Fatal(err)
		}
		old, err := os.ReadFile(replayed)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(old, written) || !sameLines(old, written) {
			t.Error("the run should write the macro-by-macro lines, in a different order")
		}
	})

	t.Run("a config assembled in Go, with no source to expand, runs macro by macro", func(t *testing.T) {
		config, err := LoadConfig("../../cfg/example_macro_config.yaml")
		if err != nil {
			t.Fatal(err)
		}
		config.source = nil
		result, err := RunToStorage(config)
		if err != nil {
			t.Fatal(err)
		}
		reference, err := RunMacros(LoadApiRunConfigFromYaml("../../cfg/example_macro_config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		assertStoragesEqual(t, "assembled in Go", result.Storage, reference)
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

// sameLines reports that two logs hold the same lines, whatever their order.
func sameLines(a, b []byte) bool {
	count := map[string]int{}
	for _, line := range strings.Split(string(a), "\n") {
		count[line]++
	}
	for _, line := range strings.Split(string(b), "\n") {
		count[line]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}
