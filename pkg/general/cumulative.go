package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// CumulativeIteration sums the new state value to the previous state
// value of the provided iteration for all iterations.
type CumulativeIteration struct {
	Iteration simulator.Iteration
}

func (c *CumulativeIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *CumulativeIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	new := c.Iteration.Iterate(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	floats.Add(new, stateHistories[partitionIndex].Values.RawRowView(0))
	return new
}
