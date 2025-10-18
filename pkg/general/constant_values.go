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
