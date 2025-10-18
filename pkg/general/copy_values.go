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
