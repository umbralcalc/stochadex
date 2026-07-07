package amr

import (
	"math"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// without bloating BuildStub's signature. BuildStub takes no seed, so we vary the
// ensemble via SetGlobalSeed.
func runStubOverride(
	t *testing.T,
	prescribingRate float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(prescribingRate, numSteps)
	gen.SetGlobalSeed(seed)
	if override != nil {
		override(gen)
	}
	settings, implementations := gen.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// meanTotalColonisation averages the total colonised fraction (S + R) over the
// back half of the run (a rough steady-state window).
func meanTotalColonisation(rows [][]float64) float64 {
	start := len(rows) / 2
	var sum float64
	var n int
	for i := start; i < len(rows); i++ {
		sum += rows[i][0] + rows[i][1]
		n++
	}
	return sum / float64(n)
}

// ensembleResistant averages the steady-state resistant fraction over nMembers
// seeds under a fixed prescribing rate and override.
func ensembleResistant(
	t *testing.T,
	prescribingRate float64,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, prescribingRate, numSteps, uint64(7000+m), override)
		sum += meanResistantFraction(store.GetValues("colonisation"))
	}
	return sum / float64(nMembers)
}

// TestAMRExpectedBehaviour is the expected-behaviour suite: each subtest name
// states a response the model is claimed to produce, checked here. Together they
// specify how the stewardship lever acts (decision path) and why the model
// should transfer off-sample (structural drivers).
func TestAMRExpectedBehaviour(t *testing.T) {
	const steps, nMembers = 200, 8

	// ----- Decision-path / mechanism (the actionable stewardship lever) -----

	// Explainability of the decision lever: prescribing pressure raises resistance
	// ONLY through the selection term. With selection switched off, cephalosporin
	// use has no pathway to favour the resistant strain, so low and high
	// prescribing give the same resistant fraction. This pins down *why* the
	// stewardship lever works — a credibility-critical causal claim.
	t.Run("prescribing_acts_only_through_selection", func(t *testing.T) {
		noSelection := func(g *simulator.ConfigGenerator) {
			g.GetPartition("colonisation").Params.Map["selection_coefficient"] = []float64{0.0}
		}
		lowR := ensembleResistant(t, 0.02, steps, nMembers, noSelection)
		highR := ensembleResistant(t, 0.8, steps, nMembers, noSelection)
		if math.Abs(highR-lowR) > 0.01 {
			t.Fatalf("with selection off, prescribing should not move resistance: "+
				"low(0.02)=%.4f high(0.8)=%.4f (Δ=%.4f)", lowR, highR, math.Abs(highR-lowR))
		}

		// Sanity: with selection ON (the default), the same prescribing sweep DOES
		// move resistance — confirming the switched-off result above is meaningful.
		lowOn := ensembleResistant(t, 0.02, steps, nMembers, nil)
		highOn := ensembleResistant(t, 0.8, steps, nMembers, nil)
		if !(highOn-lowOn > math.Abs(highR-lowR)) {
			t.Fatalf("with selection on, prescribing should move resistance more than with it off: "+
				"on Δ=%.4f off Δ=%.4f", highOn-lowOn, math.Abs(highR-lowR))
		}
	})

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	// A higher fitness cost of resistance makes the resistant strain revert faster
	// to susceptible, lowering its steady-state prevalence even under prescribing.
	t.Run("higher_fitness_cost_lowers_resistance", func(t *testing.T) {
		base := ensembleResistant(t, BaselinePrescribingRate, steps, nMembers, nil)
		costly := ensembleResistant(t, BaselinePrescribingRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
			g.GetPartition("colonisation").Params.Map["fitness_cost"] = []float64{0.15}
		})
		if !(costly < base) {
			t.Fatalf("expected a higher fitness cost to lower resistance: "+
				"base=%.4f costly(0.15)=%.4f", base, costly)
		}
	})

	// A higher within-hospital transmission coefficient spreads both strains to
	// uncolonised patients, raising total colonisation.
	t.Run("higher_transmission_raises_total_colonisation", func(t *testing.T) {
		base, high := 0.0, 0.0
		for m := 0; m < nMembers; m++ {
			base += meanTotalColonisation(runStubOverride(t, BaselinePrescribingRate, steps, uint64(7000+m), nil).GetValues("colonisation"))
			high += meanTotalColonisation(runStubOverride(t, BaselinePrescribingRate, steps, uint64(7000+m), func(g *simulator.ConfigGenerator) {
				g.GetPartition("colonisation").Params.Map["transmission_rate"] = []float64{0.09}
			}).GetValues("colonisation"))
		}
		base /= float64(nMembers)
		high /= float64(nMembers)
		if !(high > base) {
			t.Fatalf("expected higher transmission to raise total colonisation: "+
				"base=%.4f high(0.09)=%.4f", base, high)
		}
	})

	// The colonisation → infection outcome path: raising the per-patient infection
	// probability lifts the bloodstream-infection burden for a given colonisation
	// load. This is the (state) → outcome sensitivity a clinician cares about.
	t.Run("higher_infection_probability_raises_bsi", func(t *testing.T) {
		base, high := 0.0, 0.0
		for m := 0; m < nMembers; m++ {
			base += totalResistantBSI(runStubOverride(t, BaselinePrescribingRate, steps, uint64(7000+m), nil).GetValues("infection"))
			high += totalResistantBSI(runStubOverride(t, BaselinePrescribingRate, steps, uint64(7000+m), func(g *simulator.ConfigGenerator) {
				g.GetPartition("infection").Params.Map["infection_probability"] = []float64{0.03}
			}).GetValues("infection"))
		}
		base /= float64(nMembers)
		high /= float64(nMembers)
		if !(high > base) {
			t.Fatalf("expected higher infection probability to raise resistant BSI: "+
				"base=%.1f high(0.03)=%.1f", base, high)
		}
	})
}
