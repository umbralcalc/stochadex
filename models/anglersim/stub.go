package anglersim

import "github.com/umbralcalc/stochadex/pkg/simulator"

// Default generative parameters for the flagship scenario: a single brown trout
// electrofishing site under climate-driven forcing. These are illustrative values
// (broadly the constants used in the downstream Ricker demonstration), NOT a live
// posterior — data ingestion (NFPD density series + EA hydrology / water quality)
// and simulation-based calibration live in the downstream repo (see card.md). They
// exist so the stub runs with zero external inputs.
const (
	// Ricker population process.
	DefaultGrowthRate        = 0.5 // r0: baseline intrinsic growth rate
	DefaultDensityDependence = 1.0 // alpha: density-dependent mortality strength
	DefaultProcessNoiseSD    = 0.1 // sigma: process-noise standard deviation
	DefaultAlleeEffect       = 0.0 // gamma: 0 = standard Ricker (no depensation)

	// Covariate coefficients [beta_flow, beta_temp, beta_do]: warmer water hurts
	// (beta_temp < 0); more flow and more dissolved oxygen help (both > 0).
	DefaultBetaFlow = 0.10
	DefaultBetaTemp = -0.02
	DefaultBetaDO   = 0.05

	// Environmental covariate process baselines [flow_m3s, temperature_C, DO_mgl].
	DefaultBaselineFlow = 0.5
	DefaultBaselineTemp = 12.0
	DefaultBaselineDO   = 9.0

	// Covariate mean-reversion speeds. Temperature reverts at 0 (pure random walk
	// with drift) so the warming trend accumulates over the horizon.
	DefaultReversionFlow = 0.3
	DefaultReversionTemp = 0.0
	DefaultReversionDO   = 0.3

	// Covariate per-step Gaussian volatilities.
	DefaultVolatilityFlow = 0.05
	DefaultVolatilityTemp = 0.25
	DefaultVolatilityDO   = 0.4

	// DefaultWarmingTrend is the baseline climate scenario (no warming). It is the
	// one scientifically-interesting driver BuildStub exposes: °C added to the
	// temperature centre per year. A modest positive value (~0.05) is a plausible
	// multi-decade warming rate.
	DefaultWarmingTrend = 0.0

	// InitLogDensity seeds the population near its baseline equilibrium
	// (log((r0 + env)/alpha) ≈ log(0.76) ≈ -0.27) to minimise transient.
	InitLogDensity = -0.27

	// DefaultNumSteps is the flagship horizon in years.
	DefaultNumSteps = 80
)

// BuildStub constructs the data-free generative core of the anglersim model: an
// environmental covariate process (flow, temperature, dissolved oxygen) forces a
// stochastic Ricker brown-trout density process in log space. The warmingTrend is
// the climate-perturbation knob (°C/year added to the temperature centre; 0.0 =
// baseline) — exactly the parameter the CI test sweeps to check the model's
// headline claim: a warming climate lowers trout density.
//
// This is the generative core only — no data ingestion, no calibration, no
// inference, no management/decision layer.
//
// Partitions (declaration order):
//
//	covariates  mean-reverting flow/temp/DO + temperature warming trend
//	population  stochastic Ricker log-density forced by the covariates
func BuildStub(warmingTrend float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	covariates := &simulator.PartitionConfig{
		Name:      "covariates",
		Iteration: &ClimateCovariatesIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"baseline_levels": {DefaultBaselineFlow, DefaultBaselineTemp, DefaultBaselineDO},
			"reversion_rates": {DefaultReversionFlow, DefaultReversionTemp, DefaultReversionDO},
			"volatilities":    {DefaultVolatilityFlow, DefaultVolatilityTemp, DefaultVolatilityDO},
			"warming_trend":   {warmingTrend},
		}),
		InitStateValues:   []float64{DefaultBaselineFlow, DefaultBaselineTemp, DefaultBaselineDO},
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	population := &simulator.PartitionConfig{
		Name:      "population",
		Iteration: &RickerIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"growth_rate":            {DefaultGrowthRate},
			"density_dependence":     {DefaultDensityDependence},
			"covariate_coefficients": {DefaultBetaFlow, DefaultBetaTemp, DefaultBetaDO},
			"process_noise_sd":       {DefaultProcessNoiseSD},
			"allee_effect":           {DefaultAlleeEffect},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"covariates": {Upstream: "covariates"},
		},
		InitStateValues:   []float64{InitLogDensity},
		StateHistoryDepth: 1,
		Seed:              seed + 997,
	}

	gen := simulator.NewConfigGenerator()
	gen.SetPartition(covariates)
	gen.SetPartition(population)
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
