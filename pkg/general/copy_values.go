package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CopyValuesIteration writes a copy of the most recent state
// history values from other partitions to its own state.
type CopyValuesIteration struct {
}

func (c *CopyValuesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *CopyValuesIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	state := make([]float64, 0)
	for i, index := range params.Get("partition_indices") {
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
