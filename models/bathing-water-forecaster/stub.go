package bathingwater

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Default generative parameters for the flagship scenario: a small stretch of
// designated English bathing waters sharing one regional wet-week anomaly. These
// are illustrative values (a clean-water base rate of ~2–3% exceedances, matching
// the real data), NOT a live posterior — data ingestion (EA bathing-water sample
// records + rainfall), censored maximum-likelihood fitting, empirical-Bayes
// pooling, and the sequential-Monte-Carlo anomaly filter all live in the downstream
// repo (see card.md). They exist so the stub runs with zero external inputs.
const (
	// Exceedance geometry (counts in CFU/100ml). The statutory intestinal-
	// enterococci / E. coli style threshold is DefaultThresholdCount; a typical
	// clean site sits well below it, giving the rare (~2–3%) exceedance base rate.
	DefaultThresholdCount = 500.0
	DefaultSampleScale    = 0.8 // within-site log-normal sample scale (sigma)

	// Seasonal term (log units): a modest bathing-season swing in concentration.
	DefaultSeasonalAmplitude = 0.5
	DefaultSeasonalPhase     = 0.0
	DefaultPeriod            = 365.0

	// Shared regional anomaly (Ornstein–Uhlenbeck). Mean 0, mean-reverting at
	// DefaultAnomalyReversion; its volatility is the swept driver (see BuildStub).
	DefaultAnomalyReversion = 0.3
	DefaultAnomalyMean      = 0.0
	DefaultAnomalyInit      = 0.0

	// DefaultAnomalyVolatility is the baseline size of the shared wet-week swings.
	// It is the one scientifically-interesting driver BuildStub exposes: larger
	// regional weather variability drives more (and more coherent) exceedances.
	DefaultAnomalyVolatility = 0.5

	// DefaultNumSteps is the flagship horizon in days (~ one bathing season-year).
	DefaultNumSteps = 365
)

// siteSpec is one designated bathing site in the default coastline: its clean-water
// baseline count and how strongly the shared anomaly loads onto it.
type siteSpec struct {
	name          string
	baselineCount float64
	loading       float64
}

// DefaultSites is the illustrative coastline: three sites sharing one anomaly, from
// a clean weakly-coupled site to a more urban strongly-coupled one.
var DefaultSites = []siteSpec{
	{"site_0", 100.0, 0.7}, // clean, moderately coherent
	{"site_1", 150.0, 0.9}, // more urban, strongly coherent
	{"site_2", 90.0, 0.4},  // clean, weakly coherent (mostly local)
}

// BuildStub constructs the data-free generative core of the bathing-water model: a
// shared regional Ornstein–Uhlenbeck "wet-week" anomaly drives several bathing
// sites, each mapping its latent log-concentration to an exceedance probability
// against the statutory threshold. The anomalyVolatility is the weather-variability
// knob (the OU volatility; larger = bigger, more frequent regional wet swings) —
// exactly the parameter the CI test sweeps to check the model's headline claim: a
// more volatile regional anomaly raises the mean exceedance probability.
//
// This is the generative core only — no data ingestion, no censored-likelihood
// fitting, no pooling, no particle-filter inference, and no forecast/advisory
// decision layer.
//
// Partitions (declaration order):
//
//	anomaly   shared regional Ornstein–Uhlenbeck wet-week anomaly z(t)
//	site_0…N  per-site latent log-concentration → exceedance probability, all reading z
func BuildStub(anomalyVolatility float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	logThreshold := math.Log(DefaultThresholdCount)

	anomaly := &simulator.PartitionConfig{
		Name:      "anomaly",
		Iteration: &continuous.OrnsteinUhlenbeckIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"thetas": {DefaultAnomalyReversion},
			"mus":    {DefaultAnomalyMean},
			"sigmas": {anomalyVolatility},
		}),
		InitStateValues:   []float64{DefaultAnomalyInit},
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	gen := simulator.NewConfigGenerator()
	gen.SetPartition(anomaly)

	for _, s := range DefaultSites {
		site := &simulator.PartitionConfig{
			Name:      s.name,
			Iteration: &BathingConcentrationIteration{},
			Params: simulator.NewParams(map[string][]float64{
				"baseline":           {math.Log(s.baselineCount)},
				"seasonal_amplitude": {DefaultSeasonalAmplitude},
				"seasonal_phase":     {DefaultSeasonalPhase},
				"period":             {DefaultPeriod},
				"anomaly_loading":    {s.loading},
				"sample_scale":       {DefaultSampleScale},
				"log_threshold":      {logThreshold},
			}),
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"anomaly": {Upstream: "anomaly"},
			},
			InitStateValues:   []float64{0.0, 0.0},
			StateHistoryDepth: 1,
			Seed:              0,
		}
		gen.SetPartition(site)
	}

	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	return gen
}
