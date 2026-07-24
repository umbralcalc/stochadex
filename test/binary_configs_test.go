package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBinaryRunsExampleConfigs exercises the full CLI end-to-end — building the
// binary and running configs through it — which the in-process pkg/api tests do
// not do. Every config is data, so each resolves and runs in-process with no Go
// toolchain.
//
// Configs are run from the repository root (Dir = ".."), because some carry
// repo-relative output paths (e.g. ./nbs/data/test.log) — the working directory
// they are meant to be run from. This replaces the old test/configs_test.sh,
// which was never wired into CI.
func TestBinaryRunsExampleConfigs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build+run in -short mode")
	}
	const repoRoot = ".."
	binary := filepath.Join(t.TempDir(), "stochadex")
	build := exec.Command("go", "build", "-o", binary, "./cmd/stochadex")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI: %v\n%s", err, out)
	}

	configs := []struct {
		path string
		kind string
	}{
		{"cfg/example_config.yaml", "in-process (data-spec iterations)"},
		{"cfg/example_inference_config.yaml", "in-process (full inference as data + embedded)"},
		{"cfg/example_data_only_config.yaml", "in-process (fully data)"},
	}
	for _, config := range configs {
		t.Run(filepath.Base(config.path), func(t *testing.T) {
			cmd := exec.Command(binary, "--config", config.path)
			cmd.Dir = repoRoot
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("running %s [%s] through the binary failed: %v\n%s",
					config.path, config.kind, err, out)
			}
		})
	}
}
