package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ConstantValuesIteration leaves initial state values unchanged over time.
//
// Usage hints:
//   - Useful for fixed baselines or as a placeholder partition.
type ConstantValuesIteration struct {
}

func (c *ConstantValuesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *ConstantValuesIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return stateHistories[partitionIndex].Values.RawRowView(0)
}
