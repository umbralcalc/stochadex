package energybalancer

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The run helpers (runStub, finalEFC, meanFinalEFC, finalValue, meanFinalValue)
// and the override helpers live in behaviour.go so they can be shared with the card
// generator; the tests below exercise them.

func TestEnergyBalancerStub(t *testing.T) {
	// Standard convention: the stub must pass the test harness (NaN, state-width,
	// params-mutation, history-integrity and statefulness-residue checks).
	t.Run("harness", func(t *testing.T) {
		settings, implementations := BuildStub(0.5, 48, 42).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})

	// Structural / physical invariants of the generative core, checked on both
	// policy chains.
	t.Run("invariants", func(t *testing.T) {
		store := runStub(0.7, DefaultNumSteps, 42)

		minSoC := DefaultMinSoCFraction * DefaultEnergyCapacityMWh
		maxSoC := DefaultMaxSoCFraction * DefaultEnergyCapacityMWh

		for _, policy := range []string{"price", "carbon"} {
			battery := store.GetValues(policy + "_battery")
			if len(battery) == 0 {
				t.Fatalf("%s: no battery output", policy)
			}
			for i, row := range battery {
				soc, actualDispatch := row[0], row[1]
				// SoC stays within the operating window every step.
				if soc < minSoC-1e-6 || soc > maxSoC+1e-6 {
					t.Fatalf("%s step %d: SoC %v outside [%v, %v]", policy, i, soc, minSoC, maxSoC)
				}
				// Actual dispatch never exceeds the power rating in magnitude.
				if math.Abs(actualDispatch) > DefaultPowerRatingMW+1e-6 {
					t.Fatalf("%s step %d: |dispatch| %v exceeds rating %v", policy, i, actualDispatch, DefaultPowerRatingMW)
				}
			}

			// Cumulative EFC and CO₂ saved are non-negative and monotonically
			// non-decreasing (running accumulators of absolute/one-signed quantities).
			assertNonDecreasing(t, store.GetValues(policy+"_efc"), policy+" EFC")
			assertNonDecreasing(t, store.GetValues(policy+"_co2_saved"), policy+" CO2 saved")
		}

		// Carbon intensity stays physical (non-negative) across the run.
		for i, row := range store.GetValues("carbon_intensity") {
			if row[0] < 0 {
				t.Fatalf("step %d: negative carbon intensity %v", i, row[0])
			}
		}
	})

	// Headline generative claim (correct direction of parameter response): higher
	// renewable penetration means larger residual-demand swings, which carry both
	// the imbalance price and the carbon intensity across their arbitrage thresholds
	// more often, so both batteries cycle more. This is the reason the model exists
	// (storage value grows with intermittency). (The full set of response claims,
	// with their observed numbers, is in behaviour_test.go via ObservedBehaviour.)
	t.Run("more renewable intermittency raises battery cycling", func(t *testing.T) {
		const numSteps, nMembers = DefaultNumSteps, 12

		for _, policy := range []string{"price", "carbon"} {
			calmEFC := meanFinalEFC(policy, 0.2, numSteps, nMembers)
			windyEFC := meanFinalEFC(policy, 1.0, numSteps, nMembers)
			if !(windyEFC > calmEFC) {
				t.Fatalf("%s policy: expected higher renewable penetration to raise mean cumulative EFC: "+
					"calm(0.2)=%.3f windy(1.0)=%.3f", policy, calmEFC, windyEFC)
			}
		}
	})
}

// assertNonDecreasing fails the test if the first state component of any row is
// negative or drops below a previous row (an accumulator invariant).
func assertNonDecreasing(t *testing.T, rows [][]float64, label string) {
	t.Helper()
	prev := 0.0
	for i, row := range rows {
		if row[0] < -1e-9 {
			t.Fatalf("%s step %d: negative value %v", label, i, row[0])
		}
		if row[0] < prev-1e-9 {
			t.Fatalf("%s step %d: decreased %v -> %v", label, i, prev, row[0])
		}
		prev = row[0]
	}
}
