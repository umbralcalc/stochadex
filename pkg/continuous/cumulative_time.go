package continuous

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CumulativeTimeIteration defines an iteration which outputs
// the cumulative time which has elapsed in the simulation.
type CumulativeTimeIteration struct{}

func (c *CumulativeTimeIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *CumulativeTimeIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{timestepsHistory.Values.AtVec(0) +
		timestepsHistory.NextIncrement}
}
