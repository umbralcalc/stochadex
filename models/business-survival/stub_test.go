package bizsurvival

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The register-stock helpers (totalStock, meanBackHalfStock) and the override
// helpers live in behaviour.go so they can be shared with the card generator; the
// tests below exercise them.

// runStub runs the stub to completion and returns the recorded state history for
// the population partition.
func runStub(t *testing.T, hazardScale float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	t.Helper()
	settings, implementations := BuildStub(hazardScale, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// ensembleBackHalfStock averages meanBackHalfStock over nMembers seeds under a
// fixed hazard scale, damping the Poisson/binomial noise of a single realisation.
func ensembleBackHalfStock(t *testing.T, hazardScale float64, numSteps, nMembers int) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStub(t, hazardScale, numSteps, uint64(7000+m))
		sum += meanBackHalfStock(store.GetValues("population"))
	}
	return sum / float64(nMembers)
}

func TestBusinessSurvivalStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(DefaultPolicyHazardScale, 24, 7001).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural invariants of the generative core: every sector×age bucket is a
	// non-negative business count, and (starting from an empty register) formation
	// leaves a strictly positive standing stock.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(t, DefaultPolicyHazardScale, DefaultNumSteps, 7001)
		rows := store.GetValues("population")
		if len(rows) == 0 {
			t.Fatal("no population output")
		}
		for i, row := range rows {
			if len(row) != numSectors*NumAges {
				t.Fatalf("step %d: unexpected state width %d (want %d)", i, len(row), numSectors*NumAges)
			}
			for j, v := range row {
				if v < 0 {
					t.Fatalf("step %d: negative business count in bucket %d: %v", i, j, v)
				}
			}
		}
		if final := totalStock(rows[len(rows)-1]); !(final > 0) {
			t.Fatalf("expected formation to leave a positive register stock, got %v", final)
		}
	})

	// Headline generative claim (correct direction of parameter response): a
	// support package that cuts the monthly exit hazard (hazardScale < 1) raises
	// the standing register stock, and an adverse shock (hazardScale > 1) shrinks
	// it. Lower hazard → longer mean business lifetime → more businesses alive at
	// any moment. This is the scientific reason the model exists — a stub that
	// merely "runs" would not catch a sign error in the hazard term. Averaged over
	// an 8-member ensemble so the claim is about the distribution, not one draw.
	t.Run("lower hazard raises register stock", func(t *testing.T) {
		const nMembers = 8
		const supported, adverse = 0.85, 1.15

		supportedStock := ensembleBackHalfStock(t, supported, DefaultNumSteps, nMembers)
		adverseStock := ensembleBackHalfStock(t, adverse, DefaultNumSteps, nMembers)
		if !(supportedStock > adverseStock) {
			t.Fatalf("expected a lower exit hazard to raise register stock: "+
				"supported(scale=%.2f)=%.1f adverse(scale=%.2f)=%.1f",
				supported, supportedStock, adverse, adverseStock)
		}
	})
}
