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
	for i, index := range params.Get("partitions") {
		state = append(
			state,
			stateHistories[int(index)].Values.At(
				0,
				int(params.GetIndex("partition_state_values", i)),
			),
		)
	}
	return state
}
