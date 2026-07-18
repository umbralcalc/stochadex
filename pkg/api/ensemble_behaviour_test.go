package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsembleExpectedBehaviour is a physical expected-behaviour check on the
// config+ensemble path: a Wiener process with per-step variance v, run for t
// steps, is N(0, v*t) at the end. The sample variance across an ensemble of
// members must therefore be ~ v*t. This asserts the data-config Wiener actually
// behaves as Brownian motion (not just "runs"), through the full ensemble path.
func TestEnsembleExpectedBehaviour(t *testing.T) {
	const variance, steps, members = 2.0, 100, 300

	seeds := make([]string, members)
	for i := range seeds {
		seeds[i] = fmt.Sprintf("%d", 1000+i)
	}
	cfg := fmt.Sprintf(`main:
  partitions:
  - name: walk
    iteration: {type: wiener_process}
    params: {variances: [%g]}
    init_state_values: [0.0]
    state_history_depth: 1
    seed: 1
  simulation:
    output_condition: {type: every_step}
    output_function: {type: stdout}
    termination_condition: {type: number_of_steps, max_steps: %d}
    timestep_function: {type: constant, stepsize: 1.0}
    init_time_value: 0.0
run:
  mode: ensemble
  seeds: [%s]
`, variance, steps, strings.Join(seeds, ", "))

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	config := LoadApiRunConfigFromYaml(path)
	// Use the config's own resolved (data-spec) simulation, so members run the full
	// horizon — not a stand-in with a different step count.
	runs, err := ensembleRuns(config, &config.Main.Simulation)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != members {
		t.Fatalf("got %d members, want %d", len(runs), members)
	}

	// Collect each member's final value and compute the sample variance.
	finals := make([]float64, len(runs))
	var mean float64
	for i, run := range runs {
		values := run.Storage.GetValues("walk")
		finals[i] = values[len(values)-1][0]
		mean += finals[i]
	}
	mean /= float64(len(finals))
	var sampleVar float64
	for _, v := range finals {
		sampleVar += (v - mean) * (v - mean)
	}
	sampleVar /= float64(len(finals) - 1)

	want := variance * float64(steps) // 200
	// Sampling error on a variance estimate from ~300 draws is ~15%; allow 30%.
	if sampleVar < want*0.7 || sampleVar > want*1.3 {
		t.Errorf("ensemble final-value variance = %.1f, want ~%.1f (Brownian v*t) — "+
			"the config Wiener process is not diffusing correctly", sampleVar, want)
	}
}
