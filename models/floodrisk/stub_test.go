package floodrisk

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(t *testing.T, rainfallMultiplier float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	t.Helper()
	settings, implementations := BuildStub(rainfallMultiplier, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// peakFlow returns the maximum total river flow over the run after a spin-up
// window (to discard the transient from the initial soil-store state).
func peakFlow(rows [][]float64, spinUp int) float64 {
	peak := 0.0
	for i := spinUp; i < len(rows); i++ {
		if rows[i][1] > peak { // [soil_moisture, total_flow, fast_flow, slow_flow]
			peak = rows[i][1]
		}
	}
	return peak
}

// meanPeakFlow averages peak flow across an ensemble of independent realisations
// (varying only the rainfall seed) to damp the sampling noise in a single run.
func meanPeakFlow(t *testing.T, rainfallMultiplier float64, numSteps, nMembers, spinUp int) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStub(t, rainfallMultiplier, numSteps, uint64(1000+m))
		sum += peakFlow(store.GetValues("runoff"), spinUp)
	}
	return sum / float64(nMembers)
}

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
		store := runStub(t, 1.0, 1825, 42)

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
	// would not catch an inverted climate response.
	t.Run("wetter climate raises peak flow", func(t *testing.T) {
		const numSteps, nMembers, spinUp = 1825, 12, 60

		basePeak := meanPeakFlow(t, 1.0, numSteps, nMembers, spinUp)
		wetPeak := meanPeakFlow(t, 1.3, numSteps, nMembers, spinUp)

		if !(wetPeak > basePeak) {
			t.Fatalf("expected +30%% rainfall to raise mean peak flow: "+
				"baseline=%.2f wetter=%.2f m3/s", basePeak, wetPeak)
		}
	})
}
