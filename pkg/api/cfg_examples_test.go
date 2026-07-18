package api

import (
	"os"
	"testing"
)

// TestExampleConfigsRun guards the shipped in-process example configs against
// bit-rot: each resolves and runs with no Go toolchain. (Replaces the old
// test/configs_test.sh shell smoke; the Go-codegen path is covered by
// TestRunWithParsedArgs, and the full inference model by TestFullInferenceConfigAsData.)
func TestExampleConfigsRun(t *testing.T) {
	// Run from the repo root so a config's relative paths (e.g. a CSV source)
	// resolve as they do for the CLI.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	// Suppress the configs' stdout output — assert only that they run.
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()

	examples := []string{
		"cfg/example_data_only_config.yaml",
		"cfg/example_composition_config.yaml",
		"cfg/example_ensemble_config.yaml",
		"cfg/example_macro_config.yaml",
		"cfg/example_posterior_macro_config.yaml",
		"cfg/example_smc_config.yaml",
		"cfg/example_evolution_strategy_config.yaml",
		"cfg/example_data_source_config.yaml",
	}
	for _, path := range examples {
		t.Run(path, func(t *testing.T) {
			strings := LoadApiRunConfigStringsFromYaml(path)
			if !strings.IsFullyData() && len(strings.Macros) == 0 {
				t.Fatalf("%s should be runnable in-process (fully data or macros)", path)
			}
			old := os.Stdout
			os.Stdout = devnull
			defer func() { os.Stdout = old }()
			// Panics on any wiring/resolution error; completing is the assertion.
			Run(LoadApiRunConfigFromYaml(path), &SocketConfig{})
		})
	}
}
