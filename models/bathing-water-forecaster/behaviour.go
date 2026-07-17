package bathingwater

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
// the numbers the test checks, so the two can never disagree.

// runStub runs the stub to completion and returns the recorded state history for
// every partition, keyed by partition name.
func runStub(anomalyVolatility float64, numSteps int, seed uint64) *simulator.StateTimeStorage {
	settings, implementations := BuildStub(anomalyVolatility, numSteps, seed).GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// stubBuilder assembles the model, matching BuildStub's signature. The behaviour
// helpers take one rather than calling BuildStub directly so that an alternative
// assembly of the same model — notably the declarative one in declarative.yaml —
// can be put through this exact claim suite instead of a re-stated copy of it.
type stubBuilder func(anomalyVolatility float64, numSteps int, seed uint64) *simulator.ConfigGenerator

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour can vary any partition's params without
// bloating BuildStub's signature — BuildStub still exposes only the one headline
// driver (the anomaly volatility).
func runStubOverride(
	build stubBuilder,
	anomalyVolatility float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := build(anomalyVolatility, numSteps, seed)
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

// meanPExceed averages a site's exceedance probability (state index 1) over the run.
func meanPExceed(store *simulator.StateTimeStorage, site string) float64 {
	rows := store.GetValues(site)
	sum := 0.0
	for _, r := range rows {
		sum += r[1]
	}
	return sum / float64(len(rows))
}

// meanPExceedEnsemble averages meanPExceed over an ensemble of independent
// realisations (varying the anomaly seed), so each claim is about the distribution
// rather than one noisy anomaly trajectory.
func meanPExceedEnsemble(
	build stubBuilder,
	anomalyVolatility float64,
	numSteps, nMembers int,
	site string,
) float64 {
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(build, anomalyVolatility, numSteps, uint64(1000+m), nil)
		sum += meanPExceed(store, site)
	}
	return sum / float64(nMembers)
}

// meanPExceedOverride is the ensemble-mean exceedance probability for a site under
// an override, averaged over an ensemble of anomaly seeds.
func meanPExceedOverride(
	build stubBuilder,
	numSteps, nMembers int,
	site string,
	override func(*simulator.ConfigGenerator),
) float64 {
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(
			build, DefaultAnomalyVolatility, numSteps, uint64(3000+m), override)
		sum += meanPExceed(store, site)
	}
	return sum / float64(nMembers)
}

// maxPExceedOverride is the ensemble-mean of the per-run peak exceedance
// probability for a site under an override.
func maxPExceedOverride(
	build stubBuilder,
	anomalyVolatility float64,
	numSteps, nMembers int,
	site string,
	override func(*simulator.ConfigGenerator),
) float64 {
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(build, anomalyVolatility, numSteps, uint64(6000+m), override)
		rows := store.GetValues(site)
		peak := 0.0
		for _, r := range rows {
			if r[1] > peak {
				peak = r[1]
			}
		}
		sum += peak
	}
	return sum / float64(nMembers)
}

// corr computes the Pearson correlation between two equal-length series.
func corr(a, b []float64) float64 {
	n := len(a)
	var ma, mb float64
	for i := 0; i < n; i++ {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(n)
	mb /= float64(n)
	var sab, saa, sbb float64
	for i := 0; i < n; i++ {
		da, db := a[i]-ma, b[i]-mb
		sab += da * db
		saa += da * da
		sbb += db * db
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

// meanCrossSiteCorrOverride is the ensemble-mean Pearson correlation between two
// sites' exceedance-probability series under an override.
func meanCrossSiteCorrOverride(
	build stubBuilder,
	numSteps, nMembers int,
	siteA, siteB string,
	override func(*simulator.ConfigGenerator),
) float64 {
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(
			build, DefaultAnomalyVolatility, numSteps, uint64(9000+m), override)
		a := col(store.GetValues(siteA), 1)
		b := col(store.GetValues(siteB), 1)
		sum += corr(a, b)
	}
	return sum / float64(nMembers)
}

// col extracts column j from a row-major series.
func col(rows [][]float64, j int) []float64 {
	out := make([]float64, len(rows))
	for i, r := range rows {
		out[i] = r[j]
	}
	return out
}

// setSiteParam / setAnomalyParam overwrite a scalar param on a site or the anomaly.
func setSiteParam(gen *simulator.ConfigGenerator, site, key string, value float64) {
	gen.GetPartition(site).Params.Map[key] = []float64{value}
}
func setAnomalyParam(gen *simulator.ConfigGenerator, key string, value float64) {
	gen.GetPartition("anomaly").Params.Map[key] = []float64{value}
}

// ObservedBehaviour returns the model's named response claims, each with the
// ensemble numbers it produces. It is the single source of the card's "Observed
// behaviour" numbers AND the values the behaviour test asserts on, so the two can
// never disagree. Each claim's Monotone direction is what its binding test checks.
// Every claim here is a simple monotone base-vs-perturbed comparison, covering both
// the actionable management / policy levers a downstream decision-maker controls
// (pollution reduction, threshold stringency) and the structural drivers the world
// sets (anomaly volatility and persistence, regional coupling, sample scale,
// seasonality).
//
// The claim IDs match the subtest names under TestBathingWaterExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	return observedBehaviour(BuildStub)
}

// observedBehaviour computes the claims against a given assembly of the model. The
// equivalence test runs it against the declarative build so that the data-only model
// answers the same claims, measured the same way.
func observedBehaviour(build stubBuilder) []cardgen.Claim {
	const pExceed = "ensemble-mean exceedance probability"

	// Shared 365-step / 12-member clean baselines, reused across the actionable and
	// structural claims that perturb one input against them (avoids recomputing the
	// same base each time).
	const steps, nMembers = 365, 12
	baseSite0 := meanPExceedOverride(build, steps, nMembers, "site_0", nil)
	baseSite1 := meanPExceedOverride(build, steps, nMembers, "site_1", nil)

	// Decision-path: pollution reduction (halving a site's clean-water baseline count).
	cleaned := meanPExceedOverride(build, steps, nMembers, "site_1", func(g *simulator.ConfigGenerator) {
		setSiteParam(g, "site_1", "baseline", math.Log(75.0))
	})

	// Decision-path: a stricter (lower) statutory threshold.
	stricter := meanPExceedOverride(build, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
		setSiteParam(g, "site_0", "log_threshold", math.Log(250.0))
	})

	// Structural headline: a more volatile shared regional anomaly.
	calm := meanPExceedEnsemble(build, 0.3, 400, 16, "site_0")
	stormy := meanPExceedEnsemble(build, 0.8, 400, 16, "site_0")

	// Structural: stronger regional coupling (larger anomaly loading on both sites),
	// with the seasonal terms in antiphase so near-zero coupling decorrelates them.
	antiphase := func(g *simulator.ConfigGenerator) {
		setSiteParam(g, "site_0", "seasonal_phase", 0.0)
		setSiteParam(g, "site_1", "seasonal_phase", math.Pi)
	}
	weakCorr := meanCrossSiteCorrOverride(build, 600, 10, "site_0", "site_1", func(g *simulator.ConfigGenerator) {
		antiphase(g)
		setSiteParam(g, "site_0", "anomaly_loading", 0.05)
		setSiteParam(g, "site_1", "anomaly_loading", 0.05)
	})
	strongCorr := meanCrossSiteCorrOverride(build, 600, 10, "site_0", "site_1", func(g *simulator.ConfigGenerator) {
		antiphase(g)
		setSiteParam(g, "site_0", "anomaly_loading", 1.5)
		setSiteParam(g, "site_1", "anomaly_loading", 1.5)
	})

	// Structural: faster anomaly mean-reversion (higher theta) shrinks its variance.
	persistent := meanPExceedOverride(build, 400, 16, "site_0", func(g *simulator.ConfigGenerator) {
		setAnomalyParam(g, "thetas", 0.15)
	})
	fleeting := meanPExceedOverride(build, 400, 16, "site_0", func(g *simulator.ConfigGenerator) {
		setAnomalyParam(g, "thetas", 0.6)
	})

	// Structural: a larger within-site sample scale fattens the tail.
	noisier := meanPExceedOverride(build, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
		setSiteParam(g, "site_0", "sample_scale", 1.4)
	})

	// Structural: a larger seasonal amplitude raises the peak-season exceedance,
	// measured as the per-run peak with the anomaly volatility damped.
	flat := maxPExceedOverride(build, 0.1, 365, 8, "site_0", func(g *simulator.ConfigGenerator) {
		setSiteParam(g, "site_0", "seasonal_amplitude", 0.2)
	})
	swingy := maxPExceedOverride(build, 0.1, 365, 8, "site_0", func(g *simulator.ConfigGenerator) {
		setSiteParam(g, "site_0", "seasonal_amplitude", 1.2)
	})

	return []cardgen.Claim{
		{
			ID:        "pollution_reduction_lowers_exceedance_probability",
			Statement: "Pollution reduction (lower baseline) cuts a site's exceedance probability",
			Unit:      pExceed,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseSite1},
				{Label: "baseline halved", Value: cleaned},
			},
		},
		{
			ID:        "stricter_threshold_raises_flagged_exceedance_probability",
			Statement: "A stricter statutory threshold raises the flagged exceedance probability",
			Unit:      pExceed,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseSite0},
				{Label: "threshold ×0.5", Value: stricter},
			},
		},
		{
			ID:        "higher_regional_anomaly_volatility_raises_mean_exceedance",
			Statement: "Higher regional anomaly volatility raises mean exceedance (headline driver)",
			Unit:      pExceed,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "σ=0.3", Value: calm},
				{Label: "σ=0.8", Value: stormy},
			},
		},
		{
			ID:        "stronger_regional_coupling_raises_cross_site_correlation",
			Statement: "Stronger regional coupling raises the cross-site correlation of exceedance",
			Unit:      "ensemble-mean cross-site correlation",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "loading=0.05", Value: weakCorr},
				{Label: "loading=1.5", Value: strongCorr},
			},
		},
		{
			ID:        "faster_anomaly_reversion_lowers_mean_exceedance",
			Statement: "Faster anomaly mean-reversion lowers mean exceedance",
			Unit:      pExceed,
			Monotone:  -1,
			Observations: []cardgen.Observation{
				{Label: "θ=0.15", Value: persistent},
				{Label: "θ=0.6", Value: fleeting},
			},
		},
		{
			ID:        "higher_sample_scale_raises_exceedance_probability",
			Statement: "A larger within-site sample scale raises the exceedance probability",
			Unit:      pExceed,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "base", Value: baseSite0},
				{Label: "sample_scale=1.4", Value: noisier},
			},
		},
		{
			ID:        "larger_seasonal_amplitude_raises_peak_season_exceedance",
			Statement: "A larger seasonal amplitude raises the peak-season exceedance",
			Unit:      "ensemble-mean peak exceedance probability",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "amplitude=0.2", Value: flat},
				{Label: "amplitude=1.2", Value: swingy},
			},
		},
	}
}
