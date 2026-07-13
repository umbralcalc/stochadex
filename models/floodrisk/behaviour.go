package floodrisk

import (
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
func runStub(rainfallMultiplier float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
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

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params without
// bloating BuildStub's signature (which exposes only the rainfall multiplier).
func runStubOverride(
	rainfallMultiplier float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := BuildStub(rainfallMultiplier, numSteps, seed)
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
func meanPeakFlow(rainfallMultiplier float64, numSteps, nMembers, spinUp int) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStub(rainfallMultiplier, numSteps, uint64(1000+m))
		sum += peakFlow(store.GetValues("runoff"), spinUp)
	}
	return sum / float64(nMembers)
}

// meanPeakFlowOverride averages peak flow across an ensemble under a fixed
// override, damping the sampling noise in a single run.
func meanPeakFlowOverride(
	rainfallMultiplier float64,
	numSteps, nMembers, spinUp int,
	override func(*simulator.ConfigGenerator),
) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(rainfallMultiplier, numSteps, uint64(5000+m), override)
		sum += peakFlow(store.GetValues("runoff"), spinUp)
	}
	return sum / float64(nMembers)
}

// setRunoffParam / setRainfallParam overwrite a single scalar param on the runoff
// or rainfall partition.
func setRunoffParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("runoff").Params.Map[key] = []float64{value}
}
func setRainfallParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("rainfall").Params.Map[key] = []float64{value}
}

// ObservedBehaviour returns the model's named response claims with the ensemble
// numbers each produces — the single source of both the behaviour-test assertions
// and the card's "Observed behaviour" numbers. floodrisk is purely structural (its
// decision layer lives downstream), so every claim is a structural-driver response.
//
// Claim IDs match the subtest names under TestFloodRiskExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const steps, nMembers, spinUp = 730, 8, 60
	const peak = "ensemble-mean peak flow (m³/s)"

	base := meanPeakFlowOverride(1.0, steps, nMembers, spinUp, nil)

	rain115 := meanPeakFlowOverride(1.15, steps, nMembers, spinUp, nil)
	rain130 := meanPeakFlowOverride(1.3, steps, nMembers, spinUp, nil)
	persistent := meanPeakFlowOverride(1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
		setRainfallParam(g, "p_wet_given_wet", 0.95)
	})
	thirsty := meanPeakFlowOverride(1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
		setRunoffParam(g, "et_rate", 5.0)
	})
	bigger := meanPeakFlowOverride(1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
		setRunoffParam(g, "catchment_area_km2", 2.0*DefaultCatchmentAreaKm2)
	})
	spongy := meanPeakFlowOverride(1.0, steps, nMembers, spinUp, func(g *simulator.ConfigGenerator) {
		setRunoffParam(g, "field_capacity", 700.0)
	})

	return []cardgen.Claim{
		{
			ID:        "higher_rainfall_raises_peak_flow",
			Statement: "Higher rainfall raises peak flow (headline driver)",
			Unit:      peak,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "×1.0", Value: base},
				{Label: "×1.15", Value: rain115},
				{Label: "×1.3", Value: rain130},
			},
		},
		{
			ID:        "higher_wet_persistence_raises_flow",
			Statement: "Higher wet-day persistence raises flow",
			Unit:      peak,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: base},
				{Label: "p_wet_given_wet=0.95", Value: persistent},
			},
		},
		{
			ID:        "higher_evapotranspiration_lowers_flow",
			Statement: "Higher evapotranspiration lowers flow",
			Unit:      peak,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: base},
				{Label: "et_rate=5.0", Value: thirsty},
			},
		},
		{
			ID:        "larger_catchment_area_raises_flow",
			Statement: "Larger catchment area raises flow (mm→m³/s scaling)",
			Unit:      peak,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: base},
				{Label: "area ×2", Value: bigger},
			},
		},
		{
			ID:        "higher_field_capacity_lowers_peak_flow",
			Statement: "Greater soil-storage capacity lowers peak flow (\"room for water\")",
			Unit:      peak,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: base},
				{Label: "field_capacity=700mm", Value: spongy},
			},
		},
	}
}
