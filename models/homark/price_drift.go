package homark

import "github.com/umbralcalc/stochadex/pkg/simulator"

// housingPriceDrift is the reduced-form monthly log-price drift used by the
// price_drift partition. It combines three channels, matching the downstream
// homark forward model (pkg/housing/forward_values.go, PriceDriftValuesFunction):
//
//   - a baseline drift (drift_base),
//   - a mortgage-cost channel: bank_beta·(bank_rate_pct/100) — with bank_beta < 0,
//     a higher policy rate dampens price growth,
//   - a supply channel: −pipeline_beta·(market_fraction·pipeline_stock/pipeline_ref) —
//     a larger market-facing committed pipeline dampens price growth,
//   - a demand channel: demand_beta·(log_earnings − init_log_earnings) — rising
//     earnings (purchasing power) push prices up (off by default; demand_beta = 0).
//
// This is written for the catalogue rather than lifted verbatim: the downstream
// function is entangled with the ForwardOptions struct and the spine data types,
// so the data-free stub reproduces only its generative form. Coefficients are read
// from params so behaviour tests can sweep them without changing BuildStub.
func housingPriceDrift(
	driftBase, bankBeta, bankRatePct, pipelineBeta, pipelineStock,
	pipelineRef, marketFraction, demandBeta, logEarningsDelta float64,
) float64 {
	if pipelineRef <= 0 {
		pipelineRef = 1
	}
	d := driftBase + bankBeta*(bankRatePct/100.0)
	d -= pipelineBeta * (marketFraction * pipelineStock / pipelineRef)
	d += demandBeta * logEarningsDelta
	return d
}

// HousingPriceDriftFunction returns the ValuesFunctionIteration body for the
// price_drift partition. It reads the bank rate, pipeline stock and log earnings
// from the given partition indices (lag-1 state-history reads) and the drift
// coefficients from params, so every coefficient stays override-able in tests.
func HousingPriceDriftFunction(bankIdx, pipelineIdx, earningsIdx int, initLogEarnings float64) func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return func(
		params *simulator.Params,
		_ int,
		stateHistories []*simulator.StateHistory,
		_ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		bank := stateHistories[bankIdx].Values.At(0, 0)
		pipe := stateHistories[pipelineIdx].Values.At(0, 0)
		logE := stateHistories[earningsIdx].Values.At(0, 0)
		return []float64{housingPriceDrift(
			params.Get("drift_base")[0],
			params.Get("bank_beta")[0],
			bank,
			params.Get("pipeline_beta")[0],
			pipe,
			params.Get("pipeline_ref")[0],
			params.Get("market_fraction")[0],
			params.Get("demand_beta")[0],
			logE-initLogEarnings,
		)}
	}
}
