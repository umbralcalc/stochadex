package energybalancer

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ImbalancePriceIteration models the GB system imbalance price as a
// linear function of residual demand plus a stochastic noise term read
// from an upstream partition.
//
// Model:
//
//	price(t) = demand_slope * residual_demand(t) + demand_intercept + noise(t)
//
// The noise term should be supplied by an upstream
// OrnsteinUhlenbeckIteration partition (with mu=0) to capture
// mean-reverting stochastic deviations from the structural price level.
//
// Params:
//
//	demand_slope     [a]     - price sensitivity to residual demand (£/MWh per MW)
//	demand_intercept [b]     - price at zero residual demand (£/MWh)
//	demand_partition [index] - partition index of residual demand
//	noise_partition  [index] - partition index of OU noise
//
// State: [price_gbp_per_mwh]
type ImbalancePriceIteration struct{}

func (p *ImbalancePriceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *ImbalancePriceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	a := params.Map["demand_slope"][0]
	b := params.Map["demand_intercept"][0]
	demandIdx := int(params.Map["demand_partition"][0])
	noiseIdx := int(params.Map["noise_partition"][0])

	residualDemand := stateHistories[demandIdx].Values.At(0, 0)
	noise := stateHistories[noiseIdx].Values.At(0, 0)

	return []float64{a*residualDemand + b + noise}
}
