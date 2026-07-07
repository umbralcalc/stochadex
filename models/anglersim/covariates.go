package anglersim

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ClimateCovariatesIteration generates the environmental forcing the Ricker
// population process reads each step: river flow, water temperature and
// dissolved oxygen. It is a data-free stand-in for the downstream supply, which
// bootstrap-resamples these covariates from observed Environment Agency hydrology
// and water-quality records.
//
// State (and output) order is fixed: [flow_m3s, temperature_C, dissolved_oxygen_mgl].
//
// Each covariate is a mean-reverting Gaussian process about its baseline level,
// with temperature carrying an additional deterministic per-step warming drift so
// the stub can express a climate-change scenario:
//
//	x_i(t+1) = x_i(t) + reversion_i·(baseline_i − x_i(t)) + volatility_i·N(0,1)
//	                  [+ warming, for temperature]
//
// Flow and dissolved oxygen are clipped at zero (physically non-negative);
// temperature is unconstrained. The default temperature reversion is 0, so the
// warming drift accumulates into a linear warming trend rather than being pulled
// back to a fixed level; a positive reversion instead gives a warmer quasi-steady
// temperature (baseline + warming/reversion).
//
// Params:
//   - baseline_levels:  [b_flow, b_temp, b_do]   — long-run mean of each covariate
//   - reversion_rates:  [k_flow, k_temp, k_do]   — mean-reversion speed in [0,1]
//   - volatilities:     [s_flow, s_temp, s_do]   — per-step Gaussian noise scale
//   - warming_trend:    [w]                       — °C added to the temperature centre per unit time
type ClimateCovariatesIteration struct {
	rng *rand.Rand
}

// tempIndex is the fixed position of temperature in the covariate vector; it is
// the only covariate the warming trend acts on.
const tempIndex = 1

func (c *ClimateCovariatesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.rng = rand.New(rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	))
}

func (c *ClimateCovariatesIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	state := stateHistories[partitionIndex]
	baselines := params.Map["baseline_levels"]
	reversion := params.Map["reversion_rates"]
	volatility := params.Map["volatilities"]
	warming := params.Map["warming_trend"][0]

	next := make([]float64, len(baselines))
	for i := range next {
		x := state.Values.At(0, i)
		x += reversion[i]*(baselines[i]-x) + volatility[i]*c.rng.NormFloat64()
		// Temperature carries the deterministic per-step warming drift.
		if i == tempIndex {
			x += warming
		}
		// Flow and dissolved oxygen are physically non-negative.
		if i != tempIndex && x < 0 {
			x = 0
		}
		next[i] = x
	}
	return next
}
