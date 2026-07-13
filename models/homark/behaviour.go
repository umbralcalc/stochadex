package homark

import (
	"sync"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by
// two consumers: the behaviour test (which verifies each claim's direction) and
// cmd/model-graphs (which renders the observed numbers into card.md). Keeping the
// computation here — not in a _test.go file — is what lets the card show exactly
// the numbers the test checks, so the two can never disagree.

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params
// (approvals, thresholds, coefficients, rates) without bloating BuildStub's
// signature — BuildStub still exposes only the one headline driver.
func runStubOverride(
	approvalRate float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := BuildStub(approvalRate, numSteps, seed)
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

// setParam overwrites a single scalar param on a named partition.
func setParam(gen *simulator.ConfigGenerator, partition, key string, value float64) {
	gen.GetPartition(partition).Params.Map[key] = []float64{value}
}

// finalAffordability returns the price-to-earnings ratio at the end of a run.
func finalAffordability(store *simulator.StateTimeStorage) float64 {
	rows := store.GetValues("affordability")
	return rows[len(rows)-1][0]
}

// meanFinalAff ensemble-averages the final price-to-earnings ratio over nMembers
// seeds under a fixed approval rate and override, damping single-run noise.
func meanFinalAff(
	approvalRate float64,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(approvalRate, numSteps, uint64(3000+m), override)
		sum += finalAffordability(store)
	}
	return sum / float64(nMembers)
}

// meanPipelineStock ensemble-averages the pipeline stock over every step and
// nMembers seeds — the cleanest summary of how full the supply pipeline runs.
func meanPipelineStock(
	approvalRate float64,
	numSteps, nMembers int,
	override func(*simulator.ConfigGenerator),
) float64 {
	var sum float64
	var count int
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(approvalRate, numSteps, uint64(3000+m), override)
		for _, row := range store.GetValues("pipeline") {
			sum += row[0]
			count++
		}
	}
	return sum / float64(count)
}

// ObservedBehaviour returns the model's named response claims, each with the
// ensemble numbers it produces. It is the single source of the card's "Observed
// behaviour" numbers AND the values the behaviour test asserts on, so the two can
// never disagree. Each claim's Monotone direction is what its binding test checks.
// Affordability is a price-to-earnings ratio, so a *lower* value is *better*
// affordability.
//
// The claim IDs match the subtest names under TestHomarkExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const steps, nMembers = DefaultNumSteps, 8
	const pe = "ensemble-mean final price-to-earnings ratio"

	// Shared baseline at the default approval rate, reused across the structural
	// claims that perturb one input against it (avoids recomputing the same base).
	base := meanFinalAff(DefaultApprovalRate, steps, nMembers, nil)

	// Actionable supply lever: raising planning approvals from 60 to 240 units/month.
	lowApprovals := meanFinalAff(60.0, steps, nMembers, nil)
	highApprovals := meanFinalAff(240.0, steps, nMembers, nil)

	// Actionable tenure lever: cutting the market-facing delivery fraction.
	fullMarket := meanFinalAff(DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_drift", "market_fraction", 1.0)
	})
	reducedMarket := meanFinalAff(DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_drift", "market_fraction", 0.3)
	})

	// Structural drivers.
	hikedRate := meanFinalAff(DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		setParam(g, "bank_rate", "mus", 6.0)
	})
	coupledDemand := meanFinalAff(DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		setParam(g, "price_drift", "demand_beta", 0.03)
	})
	fastEarnings := meanFinalAff(DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		setParam(g, "log_earnings", "drift_coefficients", 0.006)
	})

	// Pipeline throughput invariant (mean standing stock, not affordability).
	basePipeline := meanPipelineStock(DefaultApprovalRate, steps, nMembers, nil)
	fasterPipeline := meanPipelineStock(DefaultApprovalRate, steps, nMembers, func(g *simulator.ConfigGenerator) {
		setParam(g, "pipeline", "completion_rate", 0.30)
	})

	return []cardgen.Claim{
		{
			ID:        "higher_approval_rate_improves_affordability",
			Statement: "Higher planning approvals improve affordability (headline supply lever)",
			Unit:      pe,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "approvals=60", Value: lowApprovals},
				{Label: "approvals=240", Value: highApprovals},
			},
		},
		{
			ID:        "lower_market_delivery_fraction_worsens_affordability",
			Statement: "A lower market-facing delivery fraction worsens affordability",
			Unit:      pe,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "market_fraction=1.0", Value: fullMarket},
				{Label: "market_fraction=0.3", Value: reducedMarket},
			},
		},
		{
			ID:        "higher_policy_rate_lowers_price_to_earnings",
			Statement: "A higher policy rate cools the market and lowers price-to-earnings",
			Unit:      pe,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "μ=3%", Value: base},
				{Label: "μ=6%", Value: hikedRate},
			},
		},
		{
			ID:        "stronger_demand_pressure_raises_price_to_earnings",
			Statement: "Stronger demand pressure raises price-to-earnings",
			Unit:      pe,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "demand_beta=0", Value: base},
				{Label: "demand_beta=0.03", Value: coupledDemand},
			},
		},
		{
			ID:        "higher_earnings_growth_improves_affordability",
			Statement: "Faster earnings growth improves affordability (income denominator)",
			Unit:      pe,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "drift=0.0025", Value: base},
				{Label: "drift=0.006", Value: fastEarnings},
			},
		},
		{
			ID:        "faster_pipeline_completion_lowers_pipeline_stock",
			Statement: "A faster pipeline completion rate lowers the mean pipeline stock",
			Unit:      "ensemble-mean pipeline stock (units)",
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "completion_rate=0.15", Value: basePipeline},
				{Label: "completion_rate=0.30", Value: fasterPipeline},
			},
		},
	}
}
