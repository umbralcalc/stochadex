package amr

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The run helpers (runStub) and metric helpers (meanResistantFraction,
// meanTotalColonisation, totalResistantBSI) live in behaviour.go so they can be
// shared with the card generator; the tests below exercise them.

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
		store := runStub(BaselinePrescribingRate, DefaultNumSteps)

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
	// catch a sign error in the selection term. (The full set of response claims,
	// with their observed numbers, is in behaviour_test.go via ObservedBehaviour.)
	t.Run("prescribing raises resistance", func(t *testing.T) {
		const lowRate, highRate = 0.02, 0.8

		lowStore := runStub(lowRate, DefaultNumSteps)
		highStore := runStub(highRate, DefaultNumSteps)

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
