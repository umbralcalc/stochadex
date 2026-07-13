package amr

import (
	"math"
	"sync"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by
// the behaviour test (which verifies each claim) and cmd/model-graphs (which
// renders the observed numbers into card.md). Keeping the computation here — not in
// a _test.go file — is what lets the card show exactly the numbers the test checks.

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(prescribingRate float64, numSteps int) *simulator.StateTimeStorage {
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

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run. BuildStub takes no seed, so the ensemble varies via
// SetGlobalSeed.
func runStubOverride(
	prescribingRate float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
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

// meanTotalColonisation averages the total colonised fraction (S + R) over the
// back half of the run.
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

// totalResistantBSI sums resistant bloodstream-infection counts over the run.
func totalResistantBSI(rows [][]float64) float64 {
	var total float64
	for _, row := range rows {
		total += row[1] // [susceptible_bsi_count, resistant_bsi_count]
	}
	return total
}

// ensemble averages a per-run sample over nMembers seeds under a fixed prescribing
// rate and override.
func ensemble(
	prescribingRate float64,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
	sample func(*simulator.StateTimeStorage) float64,
) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		sum += sample(runStubOverride(prescribingRate, numSteps, uint64(7000+m), override))
	}
	return sum / float64(nMembers)
}

func resistantFrac(s *simulator.StateTimeStorage) float64 {
	return meanResistantFraction(s.GetValues("colonisation"))
}
func totalColonisation(s *simulator.StateTimeStorage) float64 {
	return meanTotalColonisation(s.GetValues("colonisation"))
}
func resistantBSI(s *simulator.StateTimeStorage) float64 {
	return totalResistantBSI(s.GetValues("infection"))
}

// ObservedBehaviour returns the model's named response claims with the ensemble
// numbers each produces — the single source of both the behaviour-test assertions
// and the card's "Observed behaviour" numbers. It covers the actionable stewardship
// lever (prescribing, and the causal claim that it acts only through selection) and
// the structural drivers that earn out-of-sample credibility.
//
// Claim IDs match the subtest names under TestAMRExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const steps, nMembers = 200, 8
	const rFrac = "ensemble-mean resistant fraction"

	noSelection := func(g *simulator.ConfigGenerator) {
		g.GetPartition("colonisation").Params.Map["selection_coefficient"] = []float64{0.0}
	}

	// Prescribing sweep with selection ON (the default) — the headline lever.
	r002 := ensemble(0.02, steps, nMembers, nil, resistantFrac)
	r030 := ensemble(0.3, steps, nMembers, nil, resistantFrac)
	r080 := ensemble(0.8, steps, nMembers, nil, resistantFrac)

	// Same sweep with selection OFF — prescribing should have no pathway to act.
	offLow := ensemble(0.02, steps, nMembers, noSelection, resistantFrac)
	offHigh := ensemble(0.8, steps, nMembers, noSelection, resistantFrac)
	offDelta := math.Abs(offHigh - offLow)
	onDelta := r080 - r002

	base := ensemble(BaselinePrescribingRate, steps, nMembers, nil, resistantFrac)
	costly := ensemble(BaselinePrescribingRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		g.GetPartition("colonisation").Params.Map["fitness_cost"] = []float64{0.15}
	}, resistantFrac)

	colBase := ensemble(BaselinePrescribingRate, steps, nMembers, nil, totalColonisation)
	colHigh := ensemble(BaselinePrescribingRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		g.GetPartition("colonisation").Params.Map["transmission_rate"] = []float64{0.09}
	}, totalColonisation)

	bsiBase := ensemble(BaselinePrescribingRate, steps, nMembers, nil, resistantBSI)
	bsiHigh := ensemble(BaselinePrescribingRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		g.GetPartition("infection").Params.Map["infection_probability"] = []float64{0.03}
	}, resistantBSI)

	return []cardgen.Claim{
		{
			ID:        "prescribing_raises_resistance",
			Statement: "Higher cephalosporin prescribing raises resistance (headline lever)",
			Unit:      rFrac,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "rate 0.02", Value: r002},
				{Label: "0.3", Value: r030},
				{Label: "0.8", Value: r080},
			},
		},
		{
			ID: "prescribing_acts_only_through_selection",
			Statement: "Prescribing moves resistance only through selection " +
				"(off: no effect; on: it moves)",
			Unit:     "resistant-fraction change over the 0.02→0.8 prescribing sweep",
			Monotone: 1, // selection-on change exceeds selection-off change
			Thresholds: []cardgen.Threshold{
				// With selection off, prescribing barely moves resistance.
				{ObsIndex: 0, GreaterThan: false, Ref: 0.01, RefLabel: "0.01"},
			},
			Observations: []cardgen.Observation{
				{Label: "selection off Δ", Value: offDelta},
				{Label: "selection on Δ", Value: onDelta},
			},
		},
		{
			ID:        "higher_fitness_cost_lowers_resistance",
			Statement: "Higher fitness cost of resistance lowers resistance",
			Unit:      rFrac,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: base},
				{Label: "fitness_cost=0.15", Value: costly},
			},
		},
		{
			ID:        "higher_transmission_raises_total_colonisation",
			Statement: "Higher within-hospital transmission raises total colonisation",
			Unit:      "ensemble-mean total colonised fraction (S+R)",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: colBase},
				{Label: "transmission_rate=0.09", Value: colHigh},
			},
		},
		{
			ID:        "higher_infection_probability_raises_bsi",
			Statement: "Higher infection probability raises resistant BSI burden",
			Unit:      "ensemble-mean total resistant BSI count",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: bsiBase},
				{Label: "infection_probability=0.03", Value: bsiHigh},
			},
		},
	}
}
