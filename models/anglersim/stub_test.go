package anglersim

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The run helpers (runStub, meanFinalLogDensity, meanFinalLogDensityEnsemble) and
// the override helpers live in behaviour.go so they can be shared with the card
// generator; the tests below exercise them.

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
		store := runStub(DefaultWarmingTrend, 200, 42)

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
	// catch an inverted climate response. Averaged over an ensemble. (The full
	// set of response claims, with their observed numbers, is in behaviour_test.go
	// via ObservedBehaviour.)
	t.Run("warming climate lowers trout density", func(t *testing.T) {
		const numSteps, window, nMembers = 80, 20, 16

		baseline := meanFinalLogDensityEnsemble(0.0, numSteps, window, nMembers)
		warmed := meanFinalLogDensityEnsemble(0.08, numSteps, window, nMembers)

		if !(warmed < baseline) {
			t.Fatalf("expected warming to lower mean final log-density: "+
				"baseline=%.4f warmed(+0.08C/yr)=%.4f", baseline, warmed)
		}
	})
}
