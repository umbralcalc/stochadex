package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CopyValuesIteration copies selected values from other partitions' latest
// states into its own state.
//
// Usage hints:
//   - Provide params: "partitions" (indices) and "partition_state_values".
type CopyValuesIteration struct {
}

func (c *CopyValuesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *CopyValuesIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	state := make([]float64, 0)
	// Hoist the per-element value-index slice out of the loop (params.GetIndex is a
	// per-call map lookup); indexing it gives identical values.
	stateValues := params.Get("partition_state_values")
	for i, index := range params.Get("partitions") {
		state = append(
			state,
			stateHistories[int(index)].Values.At(
				0,
				int(stateValues[i]),
			),
		)
	}
	return state
}
