package energybalancer

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// RevenueIteration accumulates the revenue earned by battery dispatch against
// the imbalance price. Revenue is positive when discharging (selling) and
// negative when charging (buying).
//
// Revenue per step (£) = actual_dispatch_mw × price_gbp_per_mwh × dt
//
// NOTE: one-step lag — reads the previous step's battery and price states.
//
// Params:
//
//	battery_partition [index] - partition index of the battery state
//	price_partition   [index] - partition index of the imbalance price state
//
// State: [cumulative_revenue_gbp]
type RevenueIteration struct{}

func (r *RevenueIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (r *RevenueIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	batteryIdx := int(params.Map["battery_partition"][0])
	priceIdx := int(params.Map["price_partition"][0])
	dt := timestepsHistory.NextIncrement

	actualDispatch := stateHistories[batteryIdx].Values.At(0, 1) // state[1]
	price := stateHistories[priceIdx].Values.At(0, 0)
	prevRevenue := stateHistories[partitionIndex].Values.At(0, 0)

	return []float64{prevRevenue + actualDispatch*price*dt}
}
