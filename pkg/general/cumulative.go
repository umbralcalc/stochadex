package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// CumulativeIteration accumulates a provided iteration's outputs over time.
//
// Usage hints:
//   - Wrap another iteration to compute cumulative sums step-by-step.
type CumulativeIteration struct {
	Iteration simulator.Iteration
}

func (c *CumulativeIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	// Propagate configuration to the wrapped iteration so a sampler-based inner
	// (Wiener, OU, …) has its RNG and buffers initialised rather than nil.
	c.Iteration.Configure(partitionIndex, settings)
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
