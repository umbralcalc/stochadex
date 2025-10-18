// Package general provides general-purpose iteration functions and utilities
// for stochadex simulations. It includes data transformation functions,
// aggregation utilities, and flexible iteration patterns that can be
// composed to create complex simulation behaviors.
//
// Key Features:
//   - Data transformation and reduction functions
//   - Flexible function-based iterations
//   - Parameter value management and copying
//   - Constant value generation and propagation
//   - Cumulative value tracking and accumulation
//   - Embedded simulation run support
//   - History-based value extraction
//   - Event-driven value changes
//   - Collection and sorting utilities
//   - Weighted resampling algorithms
//
// Design Philosophy:
// This package emphasizes composition and flexibility, providing building
// blocks that can be combined to create sophisticated simulation behaviors.
// Functions are designed to be pure (stateless) and composable, enabling
// complex data processing pipelines within simulations.
//
// Usage Patterns:
//   - Data preprocessing and feature engineering
//   - Custom aggregation and transformation logic
//   - Parameter management and value propagation
//   - Event-driven simulation dynamics
//   - Multi-scale simulation coordination
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
