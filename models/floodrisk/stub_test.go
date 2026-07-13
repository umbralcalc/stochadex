package floodrisk

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The run helpers (runStub, peakFlow, meanPeakFlow) and the override helpers live
// in behaviour.go so they can be shared with the card generator; the tests below
// exercise them.

func TestFloodRiskStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(1.0, 60, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(1.0, 1825, 42)

		rainfall := store.GetValues("rainfall")
		if len(rainfall) == 0 {
			t.Fatal("no rainfall output")
		}
		for i, row := range rainfall {
			if row[0] < 0 {
				t.Fatalf("step %d: negative rainfall %v", i, row[0])
			}
		}

		runoff := store.GetValues("runoff")
		for i, row := range runoff {
			soil, total, fast, slow := row[0], row[1], row[2], row[3]
			// Flows are non-negative.
			if total < 0 || fast < 0 || slow < 0 {
				t.Fatalf("step %d: negative flow total=%v fast=%v slow=%v", i, total, fast, slow)
			}
			// Soil moisture stays within the physical store [0, field_capacity].
			if soil < 0 || soil > DefaultFieldCapacity+1e-6 {
				t.Fatalf("step %d: soil moisture %v outside [0, %v]", i, soil, DefaultFieldCapacity)
			}
			// Total flow is the sum of its two components.
			if d := total - (fast + slow); d < -1e-9 || d > 1e-9 {
				t.Fatalf("step %d: total flow %v != fast+slow %v", i, total, fast+slow)
			}
		}
	})

	// Headline generative claim (correct direction of parameter response):
	// wetter climate forcing (higher rainfall_multiplier) raises flood peak
	// flows. This is the reason the model exists — a stub that merely "runs"
	// would not catch an inverted climate response. (The full set of response
	// claims, with their observed numbers, is in behaviour_test.go via
	// ObservedBehaviour.)
	t.Run("wetter climate raises peak flow", func(t *testing.T) {
		const numSteps, nMembers, spinUp = 1825, 12, 60

		basePeak := meanPeakFlow(1.0, numSteps, nMembers, spinUp)
		wetPeak := meanPeakFlow(1.3, numSteps, nMembers, spinUp)

		if !(wetPeak > basePeak) {
			t.Fatalf("expected +30%% rainfall to raise mean peak flow: "+
				"baseline=%.2f wetter=%.2f m3/s", basePeak, wetPeak)
		}
	})
}
