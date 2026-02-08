package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DiscountedCumulativeIteration accumulates a provided iteration's outputs
// over time with a discount factor applied to the previous accumulated state.
// This implements the recurrence: R_k = r_k + γ * R_{k-1}, which is the
// standard discounted return when wrapping a reward-producing iteration.
//
// Usage hints:
//   - Wrap another iteration to compute discounted cumulative sums step-by-step.
//   - Provide: "discount_factor" (single float γ in [0, 1]).
type DiscountedCumulativeIteration struct {
	Iteration simulator.Iteration
}

func (d *DiscountedCumulativeIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (d *DiscountedCumulativeIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	innerOutput := d.Iteration.Iterate(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	outputValues := make([]float64, len(innerOutput))
	copy(outputValues, innerOutput)
	discountFactor := params.GetIndex("discount_factor", 0)
	previousState := stateHistories[partitionIndex].Values.RawRowView(0)
	for i := range outputValues {
		outputValues[i] += discountFactor * previousState[i]
	}
	return outputValues
}
