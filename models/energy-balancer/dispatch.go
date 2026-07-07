package energybalancer

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CarbonThresholdDispatchIteration implements a carbon-minimising dispatch policy:
//
//   - Charge (negative MW) at full power when carbon intensity < carbon_low
//     (grid is clean — absorb surplus renewable energy)
//   - Discharge (positive MW) at full power when carbon intensity > carbon_high
//     (grid is dirty — displace gas generation)
//   - Do nothing otherwise
//
// Params:
//
//	carbon_partition  [index]     - partition index of the carbon intensity state
//	carbon_high       [gCO₂/kWh] - discharge above this intensity
//	carbon_low        [gCO₂/kWh] - charge below this intensity
//	power_rating_mw   [MW]        - magnitude of charge/discharge signal
//
// State: [dispatch_mw]
type CarbonThresholdDispatchIteration struct{}

func (c *CarbonThresholdDispatchIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *CarbonThresholdDispatchIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	carbonIdx := int(params.Map["carbon_partition"][0])
	carbonHigh := params.Map["carbon_high"][0]
	carbonLow := params.Map["carbon_low"][0]
	rating := params.Map["power_rating_mw"][0]

	// actual_gco2_kwh is state index 0
	carbon := stateHistories[carbonIdx].Values.At(0, 0)

	switch {
	case carbon > carbonHigh:
		return []float64{rating} // discharge — displace gas
	case carbon < carbonLow:
		return []float64{-rating} // charge — absorb clean energy
	default:
		return []float64{0}
	}
}

// PriceThresholdDispatchIteration implements a simple price-threshold dispatch
// policy for a grid-scale battery:
//
//   - Discharge (positive MW) at full power when price > price_high
//   - Charge (negative MW) at full power when price < price_low
//   - Do nothing (0 MW) otherwise
//
// This is the canonical "energy arbitrage" strategy: buy cheap, sell dear.
//
// Params:
//
//	price_partition [index]  - partition index of the imbalance price state
//	price_high      [£/MWh]  - discharge threshold
//	price_low       [£/MWh]  - charge threshold
//	power_rating_mw [MW]     - magnitude of charge/discharge signal
//
// State: [dispatch_mw]
type PriceThresholdDispatchIteration struct{}

func (p *PriceThresholdDispatchIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *PriceThresholdDispatchIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	priceIdx := int(params.Map["price_partition"][0])
	priceHigh := params.Map["price_high"][0]
	priceLow := params.Map["price_low"][0]
	rating := params.Map["power_rating_mw"][0]

	price := stateHistories[priceIdx].Values.At(0, 0)

	switch {
	case price > priceHigh:
		return []float64{rating} // discharge
	case price < priceLow:
		return []float64{-rating} // charge
	default:
		return []float64{0}
	}
}
