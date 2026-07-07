package rugby

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

const (
	ScoreRateWidth = 4 // home_try, away_try, home_penalty, away_penalty
	CardRateWidth  = 2 // home_yellow, away_yellow

	// RateEventWidth is the full set of Cox-process rate channels
	// (score channels followed by card channels).
	RateEventWidth = ScoreRateWidth + CardRateWidth // 6

	CoeffsPerRate   = 1 + SubCovWidth                // 9 (intercept + 8 covariates)
	ScoreCoeffWidth = ScoreRateWidth * CoeffsPerRate // 36
	CardCoeffWidth  = CardRateWidth * CoeffsPerRate  // 18
)

// computeRates computes rates from coefficients, optional covariates,
// and an optional baseline using a log-linear model.
//
// With baseline (any element > 0):
//
//	rate_i = baseline_i * exp(β₀ + Σⱼ βⱼ·xⱼ)
//
// Without baseline (all zeros or empty):
//
//	rate_i = exp(β₀ + Σⱼ βⱼ·xⱼ)   (with covariates)
//	rate_i = exp(coefficients[i])   (intercept-only)
func computeRates(coefficients, covariates, baseline []float64, width int) []float64 {
	rates := make([]float64, width)
	hasBaseline := len(baseline) >= width
	hasCovariates := len(covariates) > 0 && len(coefficients) == width*CoeffsPerRate
	for i := range rates {
		logRate := 0.0
		if hasCovariates {
			offset := i * CoeffsPerRate
			logRate = coefficients[offset]
			for j := 0; j < len(covariates); j++ {
				logRate += coefficients[offset+1+j] * covariates[j]
			}
		} else if len(coefficients) > i {
			logRate = coefficients[i]
		}
		if hasBaseline && baseline[i] > 0 {
			rates[i] = baseline[i] * math.Exp(logRate)
		} else {
			rates[i] = math.Exp(logRate)
		}
	}
	return rates
}

// ScoreEventRateFunction computes rates for scoring events.
// With baseline from upstream: rate_i = baseline_i * exp(β₀ + Σβⱼxⱼ).
// Without baseline: rate_i = exp(β₀ + Σβⱼxⱼ).
func ScoreEventRateFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	coefficients := params.Get("coefficients")
	covariates, _ := params.GetOk("covariates")
	baseline := extractBaseline(params, 0, ScoreRateWidth)
	return computeRates(coefficients, covariates, baseline, ScoreRateWidth)
}

// CardEventRateFunction computes rates for card events.
// With baseline from upstream: rate_i = baseline_i * exp(β₀ + Σβⱼxⱼ).
// Without baseline: rate_i = exp(β₀ + Σβⱼxⱼ).
func CardEventRateFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	coefficients := params.Get("coefficients")
	covariates, _ := params.GetOk("covariates")
	baseline := extractBaseline(params, ScoreRateWidth, CardRateWidth)
	return computeRates(coefficients, covariates, baseline, CardRateWidth)
}

// extractBaseline reads the full baseline_rates vector from params and
// extracts the slice starting at offset with the given width. Returns nil
// if no baseline is available.
func extractBaseline(params *simulator.Params, offset, width int) []float64 {
	full, ok := params.GetOk("baseline")
	if !ok || len(full) < offset+width {
		return nil
	}
	return full[offset : offset+width]
}

// NewScoreRatesPartition creates a partition that computes scoring event
// rates from coefficients. If withCovariates is true, wires upstream
// covariates from the "sub_covariates" partition.
func NewScoreRatesPartition(coefficients []float64) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name: "score_rates",
		Iteration: &general.ValuesFunctionIteration{
			Function: ScoreEventRateFunction,
		},
		Params: simulator.NewParams(map[string][]float64{
			"coefficients": coefficients,
		}),
		InitStateValues:   make([]float64, ScoreRateWidth),
		StateHistoryDepth: 1,
		Seed:              0,
	}
}

// NewCardRatesPartition creates a partition that computes card event
// rates from coefficients.
func NewCardRatesPartition(coefficients []float64) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name: "card_rates",
		Iteration: &general.ValuesFunctionIteration{
			Function: CardEventRateFunction,
		},
		Params: simulator.NewParams(map[string][]float64{
			"coefficients": coefficients,
		}),
		InitStateValues:   make([]float64, CardRateWidth),
		StateHistoryDepth: 1,
		Seed:              0,
	}
}
