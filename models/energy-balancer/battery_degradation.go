package energybalancer

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BatteryDegradationIteration accumulates equivalent full cycles (EFC).
//
// EFC per step = |actual_dispatch_mw * dt| / (2 * energy_capacity_mwh)
//
// NOTE: one-step lag — reads previous step's battery state.
//
// Params:
//
//	battery_partition     [index] - partition index of the battery state
//	energy_capacity_mwh   [MWh]   - usable energy capacity
//
// State: [cumulative_efc]
type BatteryDegradationIteration struct{}

func (b *BatteryDegradationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (b *BatteryDegradationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	batteryIdx := int(params.Map["battery_partition"][0])
	capacity := params.Map["energy_capacity_mwh"][0]
	dt := timestepsHistory.NextIncrement

	// actual_dispatch_mw is state index 1 of the battery partition
	actualDispatch := stateHistories[batteryIdx].Values.At(0, 1)
	prevEFC := stateHistories[partitionIndex].Values.At(0, 0)

	efcPerStep := math.Abs(actualDispatch*dt) / (2.0 * capacity)

	return []float64{prevEFC + efcPerStep}
}
