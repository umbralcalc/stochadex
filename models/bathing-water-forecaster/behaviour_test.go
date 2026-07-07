package bathingwater

import (
	"math"
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStubOverride runs the stub with an override hook applied to the generated
// config before the run, so a behaviour test can vary any partition's params
// without bloating BuildStub's signature — BuildStub still exposes only the one
// headline driver (the anomaly volatility).
func runStubOverride(
	t *testing.T,
	anomalyVolatility float64,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
	gen := BuildStub(anomalyVolatility, numSteps, seed)
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

// meanPExceedOverride is the ensemble-mean exceedance probability for a site under
// an override, averaged over an ensemble of anomaly seeds.
func meanPExceedOverride(
	t *testing.T,
	numSteps, nMembers int,
	site string,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, DefaultAnomalyVolatility, numSteps, uint64(3000+m), override)
		sum += meanPExceed(store, site)
	}
	return sum / float64(nMembers)
}

// maxPExceedOverride is the ensemble-mean of the per-run peak exceedance
// probability for a site under an override.
func maxPExceedOverride(
	t *testing.T,
	anomalyVolatility float64,
	numSteps, nMembers int,
	site string,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, anomalyVolatility, numSteps, uint64(6000+m), override)
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
	t *testing.T,
	numSteps, nMembers int,
	siteA, siteB string,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	sum := 0.0
	for m := 0; m < nMembers; m++ {
		store := runStubOverride(t, DefaultAnomalyVolatility, numSteps, uint64(9000+m), override)
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

// TestBathingWaterExpectedBehaviour is the expected-behaviour suite: each subtest
// name is a plain-language response claim, and the body varies one input and
// asserts the output moves as the name says. It covers both the actionable levers a
// downstream decision-maker controls (pollution reduction lowering a site's
// baseline; regulatory threshold stringency) and the structural drivers the world
// sets (weather-anomaly volatility and persistence, regional coupling, sample-scale
// heterogeneity, seasonality) whose correct signs earn the model out-of-sample
// credibility. The forecast/advisory decision layer itself lives downstream.
func TestBathingWaterExpectedBehaviour(t *testing.T) {
	// --- Decision-path responses (actionable management / policy levers) ---

	// Pollution reduction (sewer / agricultural-runoff investment) lowers a site's
	// clean-water baseline concentration, moving it further below the threshold and
	// cutting its exceedance probability. This is the core investment lever.
	t.Run("pollution_reduction_lowers_exceedance_probability", func(t *testing.T) {
		const steps, nMembers = 365, 12
		base := meanPExceedOverride(t, steps, nMembers, "site_1", nil)
		cleaned := meanPExceedOverride(t, steps, nMembers, "site_1", func(g *simulator.ConfigGenerator) {
			// Halve the baseline count (a large pollution-input reduction).
			setSiteParam(g, "site_1", "baseline", math.Log(75.0))
		})
		if !(cleaned < base) {
			t.Fatalf("expected pollution reduction to lower exceedance probability: "+
				"base=%.4f cleaned=%.4f", base, cleaned)
		}
	})

	// A stricter (lower) statutory threshold classifies more samples as exceedances,
	// so the flagged exceedance probability rises. This is the regulatory-stringency
	// policy lever.
	t.Run("stricter_threshold_raises_flagged_exceedance_probability", func(t *testing.T) {
		const steps, nMembers = 365, 12
		base := meanPExceedOverride(t, steps, nMembers, "site_0", nil)
		stricter := meanPExceedOverride(t, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
			setSiteParam(g, "site_0", "log_threshold", math.Log(250.0)) // half the default cut
		})
		if !(stricter > base) {
			t.Fatalf("expected a stricter threshold to raise flagged exceedance: "+
				"base=%.4f stricter=%.4f", base, stricter)
		}
	})

	// --- Structural-driver responses (non-actionable levers the world sets) ---

	// A more volatile shared anomaly raises the mean exceedance probability at a
	// below-threshold site (the headline claim): bigger wet-week swings push the
	// rare tail up more often than the dry side pulls it down.
	t.Run("higher_regional_anomaly_volatility_raises_mean_exceedance", func(t *testing.T) {
		const steps, nMembers = 400, 16
		calm := meanPExceedEnsemble(t, 0.3, steps, nMembers, "site_0")
		stormy := meanPExceedEnsemble(t, 0.8, steps, nMembers, "site_0")
		if !(stormy > calm) {
			t.Fatalf("expected higher anomaly volatility to raise mean exceedance: "+
				"calm=%.4f stormy=%.4f", calm, stormy)
		}
	})

	// Stronger regional coupling (larger anomaly loading on both sites) makes their
	// exceedance probabilities move together — the distinctive "one latent process
	// driving a whole coastline" property. The seasonal terms are put in antiphase
	// so that at near-zero coupling the sites are decorrelated, isolating the effect
	// of the shared anomaly.
	t.Run("stronger_regional_coupling_raises_cross_site_correlation", func(t *testing.T) {
		const steps, nMembers = 600, 10
		antiphase := func(g *simulator.ConfigGenerator) {
			setSiteParam(g, "site_0", "seasonal_phase", 0.0)
			setSiteParam(g, "site_1", "seasonal_phase", math.Pi)
		}
		weak := meanCrossSiteCorrOverride(t, steps, nMembers, "site_0", "site_1", func(g *simulator.ConfigGenerator) {
			antiphase(g)
			setSiteParam(g, "site_0", "anomaly_loading", 0.05)
			setSiteParam(g, "site_1", "anomaly_loading", 0.05)
		})
		strong := meanCrossSiteCorrOverride(t, steps, nMembers, "site_0", "site_1", func(g *simulator.ConfigGenerator) {
			antiphase(g)
			setSiteParam(g, "site_0", "anomaly_loading", 1.5)
			setSiteParam(g, "site_1", "anomaly_loading", 1.5)
		})
		if !(strong > weak) {
			t.Fatalf("expected stronger coupling to raise cross-site correlation: "+
				"weak=%.4f strong=%.4f", weak, strong)
		}
	})

	// Faster anomaly mean-reversion (higher theta) shrinks the stationary variance
	// of the shared wet-week anomaly, so its excursions are smaller and the mean
	// exceedance probability of a below-threshold site falls.
	t.Run("faster_anomaly_reversion_lowers_mean_exceedance", func(t *testing.T) {
		const steps, nMembers = 400, 16
		persistent := meanPExceedOverride(t, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
			setAnomalyParam(g, "thetas", 0.15)
		})
		fleeting := meanPExceedOverride(t, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
			setAnomalyParam(g, "thetas", 0.6)
		})
		if !(fleeting < persistent) {
			t.Fatalf("expected faster reversion to lower mean exceedance: "+
				"persistent=%.4f fleeting=%.4f", persistent, fleeting)
		}
	})

	// A larger within-site sample scale (more heterogeneity between individual
	// samples) fattens the tail above the threshold for a normally-clean site, so
	// its exceedance probability rises.
	t.Run("higher_sample_scale_raises_exceedance_probability", func(t *testing.T) {
		const steps, nMembers = 365, 12
		base := meanPExceedOverride(t, steps, nMembers, "site_0", nil)
		noisier := meanPExceedOverride(t, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
			setSiteParam(g, "site_0", "sample_scale", 1.4)
		})
		if !(noisier > base) {
			t.Fatalf("expected a larger sample scale to raise exceedance probability: "+
				"base=%.4f noisier=%.4f", base, noisier)
		}
	})

	// A larger seasonal amplitude raises the peak-season exceedance probability: the
	// summer bathing peak drives concentration higher when the season swings wider.
	// Measured as the per-run peak, with the anomaly volatility damped so the
	// deterministic seasonal term dominates.
	t.Run("larger_seasonal_amplitude_raises_peak_season_exceedance", func(t *testing.T) {
		const steps, nMembers = 365, 8
		flat := maxPExceedOverride(t, 0.1, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
			setSiteParam(g, "site_0", "seasonal_amplitude", 0.2)
		})
		swingy := maxPExceedOverride(t, 0.1, steps, nMembers, "site_0", func(g *simulator.ConfigGenerator) {
			setSiteParam(g, "site_0", "seasonal_amplitude", 1.2)
		})
		if !(swingy > flat) {
			t.Fatalf("expected a larger seasonal amplitude to raise peak-season exceedance: "+
				"flat=%.4f swingy=%.4f", flat, swingy)
		}
	})
}
