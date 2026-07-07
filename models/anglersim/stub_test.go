package anglersim

import (
	"math"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(t *testing.T, warmingTrend float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	t.Helper()
	settings, implementations := BuildStub(warmingTrend, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// meanFinalLogDensity averages the population log-density over the final window of
// a run, damping the per-step process noise so the level is about the trajectory,
// not one noisy year.
func meanFinalLogDensity(rows [][]float64, window int) float64 {
	if window > len(rows) {
		window = len(rows)
	}
	sum := 0.0
	for i := len(rows) - window; i < len(rows); i++ {
		sum += rows[i][0]
	}
	return sum / float64(window)
}

// meanFinalLogDensityEnsemble averages meanFinalLogDensity across an ensemble of
// independent realisations (varying the seed) to make each claim about the
// distribution rather than a single trajectory.
func meanFinalLogDensityEnsemble(t *testing.T, warmingTrend float64, numSteps, window, nMembers int) float64 {
	t.Helper()
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStub(t, warmingTrend, numSteps, uint64(1000+m))
		sum += meanFinalLogDensity(store.GetValues("population"), window)
	}
	return sum / float64(nMembers)
}

func TestAnglersimStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultWarmingTrend, 60, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(t, DefaultWarmingTrend, 200, 42)

		covariates := store.GetValues("covariates")
		if len(covariates) == 0 {
			t.Fatal("no covariate output")
		}
		for i, row := range covariates {
			flow, temp, do := row[0], row[1], row[2]
			// Flow and dissolved oxygen are physically non-negative.
			if flow < 0 || do < 0 {
				t.Fatalf("step %d: negative covariate flow=%v do=%v", i, flow, do)
			}
			// All covariates stay finite.
			if math.IsNaN(temp) || math.IsInf(temp, 0) {
				t.Fatalf("step %d: non-finite temperature %v", i, temp)
			}
		}

		population := store.GetValues("population")
		for i, row := range population {
			// Log-density stays finite (no NaN / ±Inf divergence).
			if math.IsNaN(row[0]) || math.IsInf(row[0], 0) {
				t.Fatalf("step %d: non-finite log-density %v", i, row[0])
			}
		}
	})

	// Headline generative claim (correct direction of parameter response): a
	// warming climate (higher warming trend on water temperature) lowers brown
	// trout density, because the temperature covariate coefficient is negative.
	// This is the reason the model exists — a stub that merely "runs" would not
	// catch an inverted climate response. Averaged over an ensemble.
	t.Run("warming climate lowers trout density", func(t *testing.T) {
		const numSteps, window, nMembers = 80, 20, 16

		baseline := meanFinalLogDensityEnsemble(t, 0.0, numSteps, window, nMembers)
		warmed := meanFinalLogDensityEnsemble(t, 0.08, numSteps, window, nMembers)

		if !(warmed < baseline) {
			t.Fatalf("expected warming to lower mean final log-density: "+
				"baseline=%.4f warmed(+0.08C/yr)=%.4f", baseline, warmed)
		}
	})
}
