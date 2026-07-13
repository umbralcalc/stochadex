package measles

import (
	"math"
	"sync"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by
// two consumers: the behaviour test (which verifies each claim's direction) and
// cmd/model-graphs (which renders the observed numbers into card.md). Keeping the
// computation here — not in a _test.go file — is what lets the card show exactly
// the numbers the test asserts on, so the two can never disagree.

// stepAll drives a coordinator to termination.
func stepAll(coordinator *simulator.PartitionCoordinator) {
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
}

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(mmr2Coverage float64, maxGenerations int, seed uint64) *simulator.StateTimeStorage {
	settings, implementations := BuildStub(mmr2Coverage, maxGenerations, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	stepAll(coordinator)
	return store
}

// runScenarioOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params (R0,
// dispersion, the importation band, the per-UTLA surface) without bloating
// BuildStub's signature — BuildStub still exposes only the one headline driver.
func runScenarioOverride(
	mmr2Coverage float64,
	maxGenerations int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := BuildStub(mmr2Coverage, maxGenerations, seed)
	if override != nil {
		override(gen)
	}
	settings, implementations := gen.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	stepAll(coordinator)
	return store
}

// totalCases sums the final cumulative case count across all UTLAs (the national
// total for a scenario). The outbreaks state is [infectious_1..N, cumulative_1..N].
func totalCases(store *simulator.StateTimeStorage) float64 {
	rows := store.GetValues("outbreaks")
	last := rows[len(rows)-1]
	n := len(last) / 2
	var sum float64
	for i := 0; i < n; i++ {
		sum += last[n+i]
	}
	return sum
}

// meanTotalCases ensemble-averages the national total across nScenarios seeds (each
// seed draws its own shared national importation total M).
func meanTotalCases(mmr2Coverage float64, maxGenerations, nScenarios int) float64 {
	var sum float64
	for k := 0; k < nScenarios; k++ {
		sum += totalCases(runStub(mmr2Coverage, maxGenerations, uint64(1000+k)))
	}
	return sum / float64(nScenarios)
}

// meanTotalOverride ensemble-averages the national total across nScenarios seeds
// under a fixed coverage (the illustrative baseline) and override.
func meanTotalOverride(
	maxGenerations, nScenarios int,
	override func(*simulator.ConfigGenerator),
) float64 {
	var sum float64
	for k := 0; k < nScenarios; k++ {
		sum += totalCases(runScenarioOverride(DefaultMMR2Coverage, maxGenerations, uint64(4000+k), override))
	}
	return sum / float64(nScenarios)
}

// setParam overwrites a single scalar param on a named partition.
func setParam(gen *simulator.ConfigGenerator, partition, key string, value float64) {
	gen.GetPartition(partition).Params.Map[key] = []float64{value}
}

// finalCumulativeVector returns the per-UTLA cumulative case counts at the end of a
// run.
func finalCumulativeVector(store *simulator.StateTimeStorage) []float64 {
	rows := store.GetValues("outbreaks")
	last := rows[len(rows)-1]
	n := len(last) / 2
	out := make([]float64, n)
	copy(out, last[n:])
	return out
}

// meanCumulativeVector ensemble-averages the per-UTLA final cumulative case counts
// across nMembers seeds (baseSeed+k), damping the branching noise in any one run.
func meanCumulativeVector(mmr2Coverage float64, maxGenerations, nMembers int, baseSeed uint64) []float64 {
	var mean []float64
	for k := 0; k < nMembers; k++ {
		cum := finalCumulativeVector(runStub(mmr2Coverage, maxGenerations, baseSeed+uint64(k)))
		if mean == nil {
			mean = make([]float64, len(cum))
		}
		for i := range cum {
			mean[i] += cum[i]
		}
	}
	for i := range mean {
		mean[i] /= float64(nMembers)
	}
	return mean
}

// susceptibilityTertileCases ensemble-averages the per-UTLA cumulative case counts,
// then returns the mean count of the bottom third vs the top third of areas ranked
// by effective susceptibility — the core causal gradient (susceptibility, not
// chance, drives where cases concentrate) as a two-point [bottom, top] comparison.
func susceptibilityTertileCases(maxGenerations, ensemble int) (bottom, top float64) {
	susceptibility, _, _ := BuildUTLASurface(DefaultMMR2Coverage)
	n := len(susceptibility)
	meanCum := meanCumulativeVector(DefaultMMR2Coverage, maxGenerations, ensemble, 6000)
	// Rank areas by descending susceptibility (simple selection sort; n is small).
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			if susceptibility[order[b]] > susceptibility[order[a]] {
				order[a], order[b] = order[b], order[a]
			}
		}
	}
	third := n / 3
	for i := 0; i < third; i++ {
		top += meanCum[order[i]]
	}
	for i := n - third; i < n; i++ {
		bottom += meanCum[order[i]]
	}
	top /= float64(third)
	bottom /= float64(third)
	return bottom, top
}

// cvNationalTotal returns the coefficient of variation of the national total across
// nScenarios seeds under the given importation override — the summary statistic for
// how over-dispersed the national total is.
func cvNationalTotal(maxGenerations, nScenarios int, override func(*simulator.ConfigGenerator)) float64 {
	xs := make([]float64, nScenarios)
	var mean float64
	for k := 0; k < nScenarios; k++ {
		xs[k] = totalCases(runScenarioOverride(DefaultMMR2Coverage, maxGenerations, uint64(5000+k), override))
		mean += xs[k]
	}
	mean /= float64(nScenarios)
	var variance float64
	for _, x := range xs {
		variance += (x - mean) * (x - mean)
	}
	variance /= float64(nScenarios)
	return math.Sqrt(variance) / mean
}

// ObservedBehaviour returns the model's named response claims, each with the
// ensemble numbers it produces. It is the single source of the card's "Observed
// behaviour" numbers AND the values the behaviour test asserts on, so the two can
// never disagree. Each claim's Monotone direction is what its binding test checks.
//
// This model is a transmission-*risk surface*: its one actionable public-health
// lever is vaccine coverage (the decision-path claim); everything else is a
// structural driver the world sets, whose correct sign earns out-of-sample
// credibility. The targeting/ranking decision layer lives downstream.
//
// The claim IDs match the subtest names under TestMeaslesExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const gens, nScenarios = DefaultMaxGenerations, 12
	const total = "ensemble-mean national total cases"

	// ----- Decision-path response (the actionable public-health lever) -----
	lowCov := meanTotalCases(0.82, gens, nScenarios)
	highCov := meanTotalCases(0.92, gens, nScenarios)

	// ----- Structural-driver responses -----
	bottomThird, topThird := susceptibilityTertileCases(gens, 40)

	baseR0 := meanTotalOverride(gens, nScenarios, func(g *simulator.ConfigGenerator) {
		setParam(g, "outbreaks", "r0", 12.0)
	})
	hotterR0 := meanTotalOverride(gens, nScenarios, func(g *simulator.ConfigGenerator) {
		setParam(g, "outbreaks", "r0", 18.0)
	})

	calm := meanTotalOverride(gens, nScenarios, func(g *simulator.ConfigGenerator) {
		setParam(g, "national_importation", "seed_low", 10.0)
		setParam(g, "national_importation", "seed_high", 30.0)
	})
	surging := meanTotalOverride(gens, nScenarios, func(g *simulator.ConfigGenerator) {
		setParam(g, "national_importation", "seed_low", 100.0)
		setParam(g, "national_importation", "seed_high", 300.0)
	})

	const cvEnsemble = 50
	sharedCV := cvNationalTotal(gens, cvEnsemble, nil) // wide band [20, 120]
	fixedCV := cvNationalTotal(gens, cvEnsemble, func(g *simulator.ConfigGenerator) {
		setParam(g, "national_importation", "seed_low", 56.0)
		setParam(g, "national_importation", "seed_high", 56.0) // fixed M ≈ band mean
	})

	return []cardgen.Claim{
		{
			ID:        "higher_vaccine_coverage_reduces_total_cases",
			Statement: "Higher vaccine coverage reduces total cases (the actionable lever)",
			Unit:      total,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "coverage 0.82", Value: lowCov},
				{Label: "coverage 0.92", Value: highCov},
			},
		},
		{
			ID:        "higher_susceptibility_areas_accumulate_more_cases",
			Statement: "Higher-susceptibility areas accumulate more cases",
			Unit:      "ensemble-mean cumulative cases per area",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "bottom third", Value: bottomThird},
				{Label: "top third", Value: topThird},
			},
		},
		{
			ID:        "higher_R0_raises_total_cases",
			Statement: "Higher basic reproduction number raises total cases",
			Unit:      total,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "R0=12", Value: baseR0},
				{Label: "R0=18", Value: hotterR0},
			},
		},
		{
			ID:        "higher_importation_pressure_raises_total_cases",
			Statement: "Higher importation pressure raises total cases",
			Unit:      total,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "seed [10,30]", Value: calm},
				{Label: "seed [100,300]", Value: surging},
			},
		},
		{
			ID:        "shared_national_latent_over_disperses_the_national_total",
			Statement: "The shared national importation latent over-disperses the national total",
			Unit:      "coefficient of variation of the national total",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "fixed M", Value: fixedCV},
				{Label: "shared latent", Value: sharedCV},
			},
		},
	}
}
