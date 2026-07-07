package energybalancer

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BatteryIteration tracks the state of charge (SoC) of a grid-scale
// battery energy storage system (BESS) given a dispatch signal.
//
// The dispatch signal is the net power in MW: positive = discharge,
// negative = charge. It is clipped to the battery's power rating and
// the SoC is updated accounting for round-trip efficiency losses.
//
// Charging efficiency is applied on the way in; discharging efficiency
// on the way out. Combined round-trip efficiency = charge_efficiency *
// discharge_efficiency.
//
// Params:
//
//	dispatch_mw          - net dispatch signal (MW); positive = discharge
//	energy_capacity_mwh  - usable energy capacity (MWh)
//	power_rating_mw      - maximum charge/discharge power (MW)
//	charge_efficiency    - one-way charging efficiency [0,1]
//	discharge_efficiency - one-way discharging efficiency [0,1]
//	min_soc_fraction     - minimum allowed SoC as fraction of capacity [0,1]
//	max_soc_fraction     - maximum allowed SoC as fraction of capacity [0,1]
//
// State: [soc_mwh, actual_dispatch_mw]
//
//	soc_mwh         - current state of charge in MWh
//	actual_dispatch_mw - actual dispatch after applying constraints
type BatteryIteration struct{}

func (b *BatteryIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (b *BatteryIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	dispatch := params.Map["dispatch_mw"][0]
	capacity := params.Map["energy_capacity_mwh"][0]
	rating := params.Map["power_rating_mw"][0]
	chargeEff := params.Map["charge_efficiency"][0]
	dischargeEff := params.Map["discharge_efficiency"][0]
	minSoC := params.Map["min_soc_fraction"][0] * capacity
	maxSoC := params.Map["max_soc_fraction"][0] * capacity
	dt := timestepsHistory.NextIncrement

	prevSoC := stateHistories[partitionIndex].Values.At(0, 0)

	// Clip dispatch to power rating
	if dispatch > rating {
		dispatch = rating
	} else if dispatch < -rating {
		dispatch = -rating
	}

	var energyDelta float64
	if dispatch >= 0 {
		// Discharging: energy leaves battery, losses on way out
		energyDelta = -dispatch * dt / dischargeEff
	} else {
		// Charging: energy enters battery, losses on way in
		energyDelta = -dispatch * dt * chargeEff
	}

	newSoC := prevSoC + energyDelta

	// Enforce SoC limits and back-calculate actual dispatch
	var actualDispatch float64
	switch {
	case newSoC < minSoC:
		// Hit floor: can't discharge this much
		actualDispatch = (prevSoC - minSoC) * dischargeEff / dt
		newSoC = minSoC
	case newSoC > maxSoC:
		// Hit ceiling: can't charge this much
		actualDispatch = -(maxSoC - prevSoC) / (chargeEff * dt)
		newSoC = maxSoC
	default:
		actualDispatch = dispatch
	}

	return []float64{newSoC, actualDispatch}
}
