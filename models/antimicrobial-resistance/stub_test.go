package amr

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(t *testing.T, prescribingRate float64, numSteps int) *simulator.StateTimeStorage {
	t.Helper()
	settings, implementations := BuildStub(prescribingRate, numSteps).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// meanResistantFraction averages the resistant colonisation fraction over the
// back half of the run (a rough steady-state window).
func meanResistantFraction(rows [][]float64) float64 {
	start := len(rows) / 2
	var sum float64
	var n int
	for i := start; i < len(rows); i++ {
		sum += rows[i][1] // [susceptible_fraction, resistant_fraction]
		n++
	}
	return sum / float64(n)
}

func TestAMRStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(BaselinePrescribingRate, 20).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural invariants of the generative core: colonisation fractions are a
	// valid partition of the patient population, and BSI counts are non-negative.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(t, BaselinePrescribingRate, DefaultNumSteps)

		colonisation := store.GetValues("colonisation")
		if len(colonisation) == 0 {
			t.Fatal("no colonisation output")
		}
		for i, row := range colonisation {
			s, r := row[0], row[1]
			if s < 0 || s > 1 || r < 0 || r > 1 {
				t.Fatalf("step %d: colonisation fraction outside [0,1]: S=%v R=%v", i, s, r)
			}
			if s+r > 1.0+1e-9 {
				t.Fatalf("step %d: S+R exceeds 1: S=%v R=%v (sum=%v)", i, s, r, s+r)
			}
		}

		infection := store.GetValues("infection")
		for i, row := range infection {
			if row[0] < 0 || row[1] < 0 {
				t.Fatalf("step %d: negative BSI count: S=%v R=%v", i, row[0], row[1])
			}
		}
	})

	// Headline generative claim (correct direction of parameter response):
	// increasing cephalosporin prescribing pressure raises the resistant
	// colonisation fraction, and with it the resistant BSI burden. This is the
	// scientific reason the model exists — a stub that merely "runs" would not
	// catch a sign error in the selection term.
	t.Run("prescribing raises resistance", func(t *testing.T) {
		const lowRate, highRate = 0.02, 0.8

		lowStore := runStub(t, lowRate, DefaultNumSteps)
		highStore := runStub(t, highRate, DefaultNumSteps)

		lowR := meanResistantFraction(lowStore.GetValues("colonisation"))
		highR := meanResistantFraction(highStore.GetValues("colonisation"))
		if !(highR > lowR) {
			t.Fatalf("expected higher prescribing to raise resistant fraction: "+
				"low(rate=%v)=%.4f high(rate=%v)=%.4f", lowRate, lowR, highRate, highR)
		}

		// The resistant BSI burden should track the resistant colonisation load.
		lowBSI := totalResistantBSI(lowStore.GetValues("infection"))
		highBSI := totalResistantBSI(highStore.GetValues("infection"))
		if !(highBSI > lowBSI) {
			t.Fatalf("expected higher prescribing to raise resistant BSI total: "+
				"low=%v high=%v", lowBSI, highBSI)
		}
	})
}

func totalResistantBSI(rows [][]float64) float64 {
	var total float64
	for _, row := range rows {
		total += row[1] // [susceptible_bsi_count, resistant_bsi_count]
	}
	return total
}
