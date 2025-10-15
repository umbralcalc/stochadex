package continuous

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CumulativeTimeIteration outputs the cumulative simulation time.
//
// Usage hints:
//   - Returns a single-element vector: current_time + dt for the next step.
//   - Useful for logging or as an input to time-dependent components.
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
