package homark

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Default generative parameters for a single-local-authority monthly housing
// model. These are illustrative values chosen so the generative core runs with
// zero external inputs — NOT calibrated posteriors. In the downstream homark repo
// the earnings / price drift–diffusion coefficients, the bank-rate path, and the
// pipeline rates are calibrated per LA against UK HPI, BoE bank rate, ONS earnings,
// and DLUHC permissions/completions data (deterministic grid + Evolution Strategy);
// see card.md and https://github.com/umbralcalc/homark.
//
// The coefficients are set so the baseline price-to-earnings ratio (~8×) drifts
// only modestly, and the swept driver (planning approvals) moves it clearly: more
// approvals build a larger market-facing pipeline, whose committed future supply
// dampens the log-price drift and so lowers the affordability ratio.
const (
	// Initial levels. Baseline average price £240k against £30k annual earnings →
	// a price-to-earnings affordability ratio of 8.0.
	DefaultInitEarnings = 30000.0
	DefaultInitPrice    = 240000.0

	// Log-earnings drift–diffusion (monthly): ~3%/yr real-terms growth with modest noise.
	DefaultEarningsDrift = 0.0025
	DefaultEarningsDiff  = 0.003

	// Log-price diffusion (monthly). Drift is supplied by the price_drift partition.
	DefaultPriceDiff = 0.010

	// Price-drift channels (monthly log-price drift). See housingPriceDrift.
	DefaultPriceDriftBase = 0.004  // baseline growth ~5%/yr before rate/supply channels
	DefaultBankBeta       = -0.02  // higher policy rate dampens price growth
	DefaultPipelineBeta   = 0.002  // larger committed pipeline dampens price growth
	DefaultPipelineRef    = 1000.0 // reference pipeline stock for the supply channel
	DefaultDemandBeta     = 0.0    // earnings→price demand coupling (off by default)
	// MarketDeliveryFraction ∈ [0,1] scales how much of the pipeline is market-facing
	// supply. 1 = all approved units count; <1 stylises tenure/affordable requirements
	// that reduce market-facing supply and so weaken the price-dampening channel.
	DefaultMarketDeliveryFraction = 1.0

	// Bank-rate Ornstein–Uhlenbeck generator (data-free stand-in for BoE bank-rate replay).
	// stationary std ≈ sigma/sqrt(2·theta); with theta=0.1 that is ≈ 0.67 pct-points.
	DefaultBankReversion  = 0.1 // theta (per month)
	DefaultBankMeanPct    = 3.0 // mu (%)
	DefaultBankVolatility = 0.3 // sigma (%)

	// Planning-supply pipeline (per-unit binomial completions + attrition each month).
	DefaultCompletionRate = 0.15  // fraction of stock completing per month
	DefaultAttritionRate  = 0.02  // fraction of remaining stock lapsing per month
	DefaultPipelineInit   = 400.0 // initial units in the pipeline
	// DefaultApprovalRate is the illustrative baseline for the one swept driver
	// (units/month entering the pipeline). BuildStub takes it as its argument.
	DefaultApprovalRate = 100.0

	// DefaultNumSteps is the flagship horizon: ten years of monthly steps.
	DefaultNumSteps = 120

	// Δ per step (months).
	stepSizeMonths = 1.0

	// Partition indices within the stub (declaration order).
	bankRatePartition      = 0
	pipelinePartition      = 1
	priceDriftPartition    = 2
	logEarningsPartition   = 3
	logPricePartition      = 4
	affordabilityPartition = 5
)

// BuildStub constructs the data-free generative core of the single-LA housing
// model: a mean-reverting bank rate and a stochastic planning-supply pipeline feed
// a reduced-form log-price drift; log price and log earnings each evolve as
// drift–diffusion SDEs, and their ratio is the price-to-earnings affordability
// index.
//
// This is the generative core only — no data ingestion, no per-LA calibration, no
// ES/grid inference, and no scenario-grid decision layer. The one scientifically-
// interesting driver, approvalRate (planning approvals, units/month entering the
// pipeline), is exactly the parameter the CI test sweeps to check the model's
// headline claim: more approvals → more market-facing committed supply → lower
// (better) affordability.
//
// Partitions (declaration order):
//
//	bank_rate      OU mean-reverting policy rate (data-free stand-in for BoE replay)
//	pipeline       stochastic planning-supply stock (completions + attrition)
//	price_drift    reduced-form log-price drift: base + bank + supply (+ demand) channels
//	log_earnings   drift–diffusion log earnings
//	log_price      drift–diffusion log price, drift wired from price_drift
//	affordability  exp(log_price − log_earnings) — the price-to-earnings ratio
func BuildStub(approvalRate float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	initLogEarnings := math.Log(DefaultInitEarnings)
	initLogPrice := math.Log(DefaultInitPrice)

	// Initial drift uses the initial bank rate, pipeline stock, and zero earnings
	// deviation (t=0), so the price_drift partition starts consistent with its inputs.
	initDrift := housingPriceDrift(
		DefaultPriceDriftBase, DefaultBankBeta, DefaultBankMeanPct,
		DefaultPipelineBeta, DefaultPipelineInit, DefaultPipelineRef,
		DefaultMarketDeliveryFraction, DefaultDemandBeta, 0.0,
	)

	gen := simulator.NewConfigGenerator()

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "bank_rate",
		Iteration: &continuous.OrnsteinUhlenbeckIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"thetas": {DefaultBankReversion},
			"mus":    {DefaultBankMeanPct},
			"sigmas": {DefaultBankVolatility},
		}),
		InitStateValues:   []float64{DefaultBankMeanPct},
		StateHistoryDepth: 1,
		Seed:              seed,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "pipeline",
		Iteration: &StochasticPipelineIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"completion_rate": {DefaultCompletionRate},
			"attrition_rate":  {DefaultAttritionRate},
			"approval_rate":   {approvalRate},
		}),
		InitStateValues:   []float64{DefaultPipelineInit},
		StateHistoryDepth: 1,
		Seed:              seed + 1,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name: "price_drift",
		Iteration: &general.ValuesFunctionIteration{
			Function: HousingPriceDriftFunction(
				bankRatePartition, pipelinePartition, logEarningsPartition, initLogEarnings,
			),
		},
		Params: simulator.NewParams(map[string][]float64{
			"drift_base":      {DefaultPriceDriftBase},
			"bank_beta":       {DefaultBankBeta},
			"pipeline_beta":   {DefaultPipelineBeta},
			"pipeline_ref":    {DefaultPipelineRef},
			"market_fraction": {DefaultMarketDeliveryFraction},
			"demand_beta":     {DefaultDemandBeta},
		}),
		InitStateValues:   []float64{initDrift},
		StateHistoryDepth: 1,
		Seed:              0,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "log_earnings",
		Iteration: &continuous.DriftDiffusionIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"drift_coefficients":     {DefaultEarningsDrift},
			"diffusion_coefficients": {DefaultEarningsDiff},
		}),
		InitStateValues:   []float64{initLogEarnings},
		StateHistoryDepth: 1,
		Seed:              seed + 2,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "log_price",
		Iteration: &continuous.DriftDiffusionIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"diffusion_coefficients": {DefaultPriceDiff},
		}),
		// price_drift feeds this step's drift in within-step (params from upstream).
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"drift_coefficients": {Upstream: "price_drift"},
		},
		InitStateValues:   []float64{initLogPrice},
		StateHistoryDepth: 1,
		Seed:              seed + 3,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "affordability",
		Iteration: &AffordabilityFromLogsIteration{},
		Params:    simulator.NewParams(map[string][]float64{}),
		ParamsAsPartitions: map[string][]string{
			"log_price_partition":    {"log_price"},
			"log_earnings_partition": {"log_earnings"},
		},
		InitStateValues:   []float64{math.Exp(initLogPrice - initLogEarnings)},
		StateHistoryDepth: 1,
		Seed:              0,
	})

	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSizeMonths},
		InitTimeValue:    0.0,
	})
	return gen
}
