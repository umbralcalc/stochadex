package energybalancer

import (
	"math"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(t *testing.T, renewablePenetration float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	t.Helper()
	settings, implementations := BuildStub(renewablePenetration, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// finalEFC returns the cumulative equivalent full cycles at the end of a run for
// the named policy chain ("price" or "carbon"): the total battery throughput,
// and the cleanest monotone measure of how hard that policy worked its battery.
func finalEFC(store *simulator.StateTimeStorage, policy string) float64 {
	rows := store.GetValues(policy + "_efc")
	return rows[len(rows)-1][0]
}

// meanFinalEFC averages cumulative EFC across an ensemble of independent
// realisations (varying only the seed) to damp the sampling noise in a single
// stochastic run.
func meanFinalEFC(t *testing.T, policy string, renewablePenetration float64, numSteps, nMembers int) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		sum += finalEFC(runStub(t, renewablePenetration, numSteps, uint64(2000+m)), policy)
	}
	return sum / float64(nMembers)
}

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
		store := runStub(t, 0.7, DefaultNumSteps, 42)

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

	// Headline generative claim (correct direction of parameter response):
	// higher renewable penetration means larger residual-demand swings, which
	// carry both the imbalance price and the carbon intensity across their
	// arbitrage thresholds more often, so both batteries cycle more — cumulative
	// EFC rises with penetration for each policy. This is the reason the model
	// exists (storage value grows with intermittency); a stub that merely "runs"
	// would not catch an inverted volatility response.
	t.Run("more renewable intermittency raises battery cycling", func(t *testing.T) {
		const numSteps, nMembers = DefaultNumSteps, 12

		for _, policy := range []string{"price", "carbon"} {
			calmEFC := meanFinalEFC(t, policy, 0.2, numSteps, nMembers)
			windyEFC := meanFinalEFC(t, policy, 1.0, numSteps, nMembers)
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
