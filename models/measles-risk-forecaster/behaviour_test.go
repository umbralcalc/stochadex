package measles

import (
	"math"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// stepAll drives a coordinator to termination.
func stepAll(coordinator *simulator.PartitionCoordinator) {
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
}

// runScenarioOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params (R0,
// dispersion, the importation band, the per-UTLA surface) without bloating
// BuildStub's signature — BuildStub still exposes only the one headline driver.
func runScenarioOverride(
	t *testing.T,
	mmr2Coverage float64,
	maxGenerations int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
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

// meanTotalOverride ensemble-averages the national total across nScenarios seeds
// under a fixed coverage and override.
func meanTotalOverride(
	t *testing.T,
	maxGenerations, nScenarios int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	for k := 0; k < nScenarios; k++ {
		sum += totalCases(runScenarioOverride(t, DefaultMMR2Coverage, maxGenerations, uint64(4000+k), override))
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

// cvNationalTotal returns the coefficient of variation of the national total across
// nScenarios seeds under the given importation override — the summary statistic for
// how over-dispersed the national total is.
func cvNationalTotal(t *testing.T, maxGenerations, nScenarios int, override func(*simulator.ConfigGenerator)) float64 {
	t.Helper()
	xs := make([]float64, nScenarios)
	var mean float64
	for k := 0; k < nScenarios; k++ {
		xs[k] = totalCases(runScenarioOverride(t, DefaultMMR2Coverage, maxGenerations, uint64(5000+k), override))
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

// TestMeaslesExpectedBehaviour is the expected-behaviour suite: each subtest name
// states, in plain language, a response the model is claimed to produce, and the
// body checks it. This model is a transmission-*risk surface* — its one actionable
// public-health lever is vaccine coverage (the decision-path claim); everything else
// is a structural driver the world sets, whose correct sign earns out-of-sample
// credibility. The targeting/ranking decision layer lives downstream.
func TestMeaslesExpectedBehaviour(t *testing.T) {
	const gens, nScenarios = DefaultMaxGenerations, 12

	// ----- Decision-path response (the actionable public-health lever) -----

	// A catch-up MMR campaign that raises coverage lowers effective susceptibility,
	// pulls more areas below the R_local = 1 threshold, and so cuts the national case
	// total. This is the one lever a downstream decision-maker controls; a wrong sign
	// here is a wrong public-health recommendation.
	t.Run("higher_vaccine_coverage_reduces_total_cases", func(t *testing.T) {
		low := meanTotalCases(t, 0.82, gens, nScenarios)
		high := meanTotalCases(t, 0.92, gens, nScenarios)
		if !(high < low) {
			t.Fatalf("expected higher coverage to reduce total cases: "+
				"low(0.82)=%.0f high(0.92)=%.0f", low, high)
		}
	})

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	// The core causal gradient: within one season, areas with higher effective
	// susceptibility accumulate more cases. This is the mechanism the whole surface
	// rests on — susceptibility, not chance, drives *where* cases concentrate.
	t.Run("higher_susceptibility_areas_accumulate_more_cases", func(t *testing.T) {
		susceptibility, _, _ := BuildUTLASurface(DefaultMMR2Coverage)
		n := len(susceptibility)
		meanCum := make([]float64, n)
		const ensemble = 40
		for k := 0; k < ensemble; k++ {
			cum := finalCumulativeVector(runStub(t, DefaultMMR2Coverage, gens, uint64(6000+k)))
			for i := 0; i < n; i++ {
				meanCum[i] += cum[i]
			}
		}
		// Compare the mean case count of the top third vs the bottom third of areas
		// ranked by susceptibility.
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		// simple selection sort by descending susceptibility (n is small)
		for a := 0; a < n; a++ {
			for b := a + 1; b < n; b++ {
				if susceptibility[order[b]] > susceptibility[order[a]] {
					order[a], order[b] = order[b], order[a]
				}
			}
		}
		third := n / 3
		var top, bottom float64
		for i := 0; i < third; i++ {
			top += meanCum[order[i]]
		}
		for i := n - third; i < n; i++ {
			bottom += meanCum[order[i]]
		}
		top /= float64(third)
		bottom /= float64(third)
		if !(top > bottom) {
			t.Fatalf("expected higher-susceptibility areas to accumulate more cases: "+
				"top-third=%.1f bottom-third=%.1f", top, bottom)
		}
	})

	// Transmissibility: a higher basic reproduction number raises R_local = R0·s in
	// every area, so more outbreaks self-sustain and the national total rises. R0 is
	// set by the pathogen and contact structure, not by policy.
	t.Run("higher_R0_raises_total_cases", func(t *testing.T) {
		base := meanTotalOverride(t, gens, nScenarios, func(g *simulator.ConfigGenerator) {
			setParam(g, "outbreaks", "r0", 12.0)
		})
		hotter := meanTotalOverride(t, gens, nScenarios, func(g *simulator.ConfigGenerator) {
			setParam(g, "outbreaks", "r0", 18.0)
		})
		if !(hotter > base) {
			t.Fatalf("expected higher R0 to raise total cases: "+
				"R0=12 -> %.0f, R0=18 -> %.0f", base, hotter)
		}
	})

	// Importation pressure: a higher / wider national seed band (more European measles
	// activity) seeds more index cases across areas, so the national total rises.
	// Importation pressure is set by upstream European incidence, not domestic policy.
	t.Run("higher_importation_pressure_raises_total_cases", func(t *testing.T) {
		calm := meanTotalOverride(t, gens, nScenarios, func(g *simulator.ConfigGenerator) {
			setParam(g, "national_importation", "seed_low", 10.0)
			setParam(g, "national_importation", "seed_high", 30.0)
		})
		surging := meanTotalOverride(t, gens, nScenarios, func(g *simulator.ConfigGenerator) {
			setParam(g, "national_importation", "seed_low", 100.0)
			setParam(g, "national_importation", "seed_high", 300.0)
		})
		if !(surging > calm) {
			t.Fatalf("expected higher importation pressure to raise total cases: "+
				"calm=%.0f surging=%.0f", calm, surging)
		}
	})

	// Co-occurrence — the reason the model is multi-partition. A shared national
	// importation latent (M drawn once, varying scenario to scenario) makes areas
	// surge together, so the national total is over-dispersed: its coefficient of
	// variation is markedly larger than under a fixed importation level (which leaves
	// only independent branching noise). This is the joint-tail behaviour a per-area
	// marginal model cannot reproduce.
	t.Run("shared_national_latent_over_disperses_the_national_total", func(t *testing.T) {
		const ensemble = 50
		shared := cvNationalTotal(t, gens, ensemble, nil) // wide band [20, 120]
		fixed := cvNationalTotal(t, gens, ensemble, func(g *simulator.ConfigGenerator) {
			setParam(g, "national_importation", "seed_low", 56.0)
			setParam(g, "national_importation", "seed_high", 56.0) // fixed M ≈ band mean
		})
		if !(shared > fixed) {
			t.Fatalf("expected the shared importation latent to over-disperse the national total: "+
				"shared-latent CV=%.3f fixed-M CV=%.3f", shared, fixed)
		}
	})
}
