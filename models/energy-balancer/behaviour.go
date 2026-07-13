package energybalancer

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
func runStub(renewablePenetration float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
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

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params without
// bloating BuildStub's signature.
func runStubOverride(
	renewablePenetration float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := BuildStub(renewablePenetration, numSteps, seed)
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

// finalValue returns the last recorded value of state component idx of the named
// partition.
func finalValue(store *simulator.StateTimeStorage, partition string, idx int) float64 {
	rows := store.GetValues(partition)
	return rows[len(rows)-1][idx]
}

// finalEFC returns the cumulative equivalent full cycles at the end of a run for
// the named policy chain ("price" or "carbon").
func finalEFC(store *simulator.StateTimeStorage, policy string) float64 {
	return finalValue(store, policy+"_efc", 0)
}

// meanFinalEFC averages cumulative EFC across an ensemble of seeds.
func meanFinalEFC(policy string, renewablePenetration float64, numSteps, nMembers int) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		sum += finalEFC(runStub(renewablePenetration, numSteps, uint64(2000+m)), policy)
	}
	return sum / float64(nMembers)
}

// meanFinalValue ensemble-averages finalValue over nMembers seeds under a fixed
// penetration and override, damping single-run noise.
func meanFinalValue(
	renewablePenetration float64,
	numSteps, nMembers int,
	partition string,
	idx int,
	override func(*simulator.ConfigGenerator),
) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(renewablePenetration, numSteps, uint64(3000+m), override)
		sum += finalValue(store, partition, idx)
	}
	return sum / float64(nMembers)
}

// setParam overwrites a single scalar param on a named partition.
func setParam(gen *simulator.ConfigGenerator, partition, key string, value float64) {
	gen.GetPartition(partition).Params.Map[key] = []float64{value}
}

// ObservedBehaviour returns the model's named response claims with the numbers each
// produces — the single source of both the behaviour-test assertions and the card's
// "Observed behaviour" numbers. It covers the actionable levers a downstream
// controls (dispatch threshold, sizing, and the (state,action)→outcome sign of
// net-seller/net-buyer trades) and the structural drivers that earn off-sample
// credibility.
//
// Claim IDs match the subtest names under TestEnergyBalancerExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const steps, nMembers, pen = 168, 8, 0.6
	const efcUnit = "ensemble-mean cumulative EFC"

	baseEFC := meanFinalValue(pen, steps, nMembers, "price_efc", 0, nil)
	baseRevenue := meanFinalValue(pen, steps, nMembers, "price_revenue", 0, nil)
	baseCarbonEFC := meanFinalValue(pen, steps, nMembers, "carbon_efc", 0, nil)

	// Headline: storage value grows with intermittency — more renewable penetration
	// raises price-policy cycling.
	pen00 := meanFinalValue(0.0, steps, nMembers, "price_efc", 0, nil)
	pen05 := meanFinalValue(0.5, steps, nMembers, "price_efc", 0, nil)
	pen10 := meanFinalValue(1.0, steps, nMembers, "price_efc", 0, nil)

	strict := meanFinalValue(pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_dispatch", "price_high", 60.0)
	})
	big := meanFinalValue(pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_battery", "energy_capacity_mwh", 400.0)
		setParam(g, "price_efc", "energy_capacity_mwh", 400.0)
	})
	leaky := meanFinalValue(pen, steps, nMembers, "price_revenue", 0, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_battery", "charge_efficiency", 0.75)
		setParam(g, "price_battery", "discharge_efficiency", 0.75)
	})
	noisy := meanFinalValue(pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_noise", "sigmas", 15.0)
	})
	steep := meanFinalValue(pen, steps, nMembers, "price_efc", 0, func(g *simulator.ConfigGenerator) {
		setParam(g, "price", "demand_slope", 0.004)
		setParam(g, "price", "demand_intercept", 35.0-0.004*DefaultResidualMeanMW)
	})
	carbonSteep := meanFinalValue(pen, steps, nMembers, "carbon_efc", 0, func(g *simulator.ConfigGenerator) {
		setParam(g, "carbon_intensity", "carbon_slope", 0.020)
		setParam(g, "carbon_intensity", "carbon_intercept", 175.0-0.020*DefaultResidualMeanMW)
	})

	// (state, action) → outcome sign claims — single seeded runs driven hard into
	// the branch (expensive vs cheap market), asserting the trade's sign.
	expensive := runStubOverride(pen, steps, 42, func(g *simulator.ConfigGenerator) {
		setParam(g, "residual_demand", "mus", 30000.0) // price ≈ £50 > £45 sell bar
		setParam(g, "residual_demand", "sigmas", 50.0)
		setParam(g, "price_noise", "sigmas", 0.5)
	})
	sellSoC := finalValue(expensive, "price_battery", 0)
	sellRevenue := finalValue(expensive, "price_revenue", 0)

	cheap := runStubOverride(pen, steps, 42, func(g *simulator.ConfigGenerator) {
		setParam(g, "residual_demand", "mus", 15000.0) // price ≈ £20 < £25 buy bar
		setParam(g, "residual_demand", "sigmas", 50.0)
		setParam(g, "price_noise", "sigmas", 0.5)
	})
	buySoC := finalValue(cheap, "price_battery", 0)
	buyRevenue := finalValue(cheap, "price_revenue", 0)

	return []cardgen.Claim{
		{
			ID:        "higher_renewable_penetration_raises_cycling",
			Statement: "Higher renewable penetration raises battery cycling (headline driver)",
			Unit:      efcUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "pen 0.0", Value: pen00},
				{Label: "0.5", Value: pen05},
				{Label: "1.0", Value: pen10},
			},
		},
		{
			ID:        "higher_discharge_threshold_reduces_price_cycling",
			Statement: "Higher discharge threshold reduces price-policy cycling",
			Unit:      efcUnit,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseEFC},
				{Label: "price_high=£60", Value: strict},
			},
		},
		{
			ID:        "larger_battery_capacity_lowers_cycle_count",
			Statement: "Larger battery capacity lowers the cycle count",
			Unit:      efcUnit,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseEFC},
				{Label: "400 MWh", Value: big},
			},
		},
		{
			ID: "persistently_expensive_market_makes_battery_net_seller",
			Statement: "A persistently expensive market drains the battery (net seller) " +
				"for positive revenue",
			Unit:     "one expensive-market run (seed 42)",
			Monotone: 0,
			Thresholds: []cardgen.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: InitialSoCMWh, RefLabel: "initial SoC"},
				{ObsIndex: 1, GreaterThan: true, Ref: 0, RefLabel: "£0"},
			},
			Observations: []cardgen.Observation{
				{Label: "final SoC (MWh)", Value: sellSoC},
				{Label: "revenue (£)", Value: sellRevenue},
			},
		},
		{
			ID: "persistently_cheap_market_makes_battery_net_buyer",
			Statement: "A persistently cheap market fills the battery (net buyer) " +
				"at a cost",
			Unit:     "one cheap-market run (seed 42)",
			Monotone: 0,
			Thresholds: []cardgen.Threshold{
				{ObsIndex: 0, GreaterThan: true, Ref: InitialSoCMWh, RefLabel: "initial SoC"},
				{ObsIndex: 1, GreaterThan: false, Ref: 0, RefLabel: "£0"},
			},
			Observations: []cardgen.Observation{
				{Label: "final SoC (MWh)", Value: buySoC},
				{Label: "revenue (£)", Value: buyRevenue},
			},
		},
		{
			ID:        "lower_round_trip_efficiency_reduces_revenue",
			Statement: "Lower round-trip efficiency reduces revenue",
			Unit:      "ensemble-mean price-policy revenue (£)",
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseRevenue},
				{Label: "η=0.75", Value: leaky},
			},
		},
		{
			ID:        "higher_price_noise_raises_cycling",
			Statement: "Higher price noise raises cycling",
			Unit:      efcUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseEFC},
				{Label: "σ=15", Value: noisy},
			},
		},
		{
			ID:        "steeper_price_sensitivity_raises_cycling",
			Statement: "Steeper price sensitivity (mean held) raises cycling",
			Unit:      efcUnit,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseEFC},
				{Label: "slope=0.004", Value: steep},
			},
		},
		{
			ID:        "higher_carbon_sensitivity_raises_carbon_cycling",
			Statement: "Steeper carbon sensitivity (mean held) raises carbon-policy cycling",
			Unit:      "ensemble-mean carbon-policy cumulative EFC",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseCarbonEFC},
				{Label: "slope=0.020", Value: carbonSteep},
			},
		},
	}
}
