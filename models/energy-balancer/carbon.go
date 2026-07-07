package energybalancer

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CarbonIntensityIteration is the generative, data-free stand-in for the
// downstream CarbonDataIteration (which replays a measured carbon-intensity CSV).
// It models grid carbon intensity as the same structural reduced form as the
// imbalance price: a linear response to residual demand plus mean-reverting
// noise. Carbon intensity rises with residual demand because dirtier marginal
// plant (gas, and historically coal) runs to meet higher net load, and falls
// when renewables dominate and net load is low.
//
// Driving both price and carbon intensity off the same residual-demand process
// reproduces their real co-movement: a low-wind period simultaneously raises the
// price and the carbon intensity.
//
// Model:
//
//	carbon(t) = carbon_slope * residual_demand(t) + carbon_intercept + noise(t)
//
// The noise term should be supplied by an upstream OU partition (mu=0).
//
// Params:
//
//	carbon_slope     [c]     - intensity sensitivity to residual demand (gCO₂/kWh per MW)
//	carbon_intercept [d]     - intensity at zero residual demand (gCO₂/kWh)
//	demand_partition [index] - partition index of residual demand
//	noise_partition  [index] - partition index of OU noise
//
// State: [carbon_gco2_kwh]
type CarbonIntensityIteration struct{}

func (c *CarbonIntensityIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *CarbonIntensityIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	slope := params.Map["carbon_slope"][0]
	intercept := params.Map["carbon_intercept"][0]
	demandIdx := int(params.Map["demand_partition"][0])
	noiseIdx := int(params.Map["noise_partition"][0])

	residualDemand := stateHistories[demandIdx].Values.At(0, 0)
	noise := stateHistories[noiseIdx].Values.At(0, 0)

	return []float64{slope*residualDemand + intercept + noise}
}
