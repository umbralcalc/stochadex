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

// ParamsTransform is a convenience transform that returns the current params
// map. Useful as a building block for transform/reduce pipelines.
func ParamsTransform(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) map[string][]float64 {
	return params.Map
}

// SumReduce reduces a map of equally sized vectors by summing them
// element-wise into a single output vector.
//
// Usage hints:
//   - Combine with NewTransformReduceFunction to compose dataflow operations.
func SumReduce(values map[string][]float64) []float64 {
	var out []float64
	for _, v := range values {
		if out == nil {
			out = make([]float64, len(v))
		}
		floats.Add(out, v)
	}
	return out
}

// NewTransformReduceFunction returns a function that first transforms the
// provided simulation context into a map of vectors, then reduces those
// vectors into a single vector.
//
// Usage hints:
//   - Compose transform/reduce pipelines to feed ValuesFunctionIteration.
func NewTransformReduceFunction(
	transform func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) map[string][]float64,
	reduce func(values map[string][]float64) []float64,
) func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []float64 {
		return reduce(transform(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		))
	}
}

// ValuesFunctionIteration provides a flexible way to compute derived values
// from simulation state and parameters using user-defined functions.
//
// This iteration type allows for custom computation logic within simulations,
// enabling feature engineering, data transformation, and complex derived
// value calculations. It's particularly useful for creating custom
// aggregation functions, feature extraction, and data preprocessing.
//
// Design Philosophy:
// The function-based approach emphasizes composition and reusability. By
// providing a pure function interface, this iteration enables:
//   - Stateless computation (no side effects)
//   - Easy testing and validation
//   - Composition with other iteration types
//   - Reusable computation logic across simulations
//
// Function Signature:
// The Function field must implement a pure mapping from simulation context
// to output values. It receives:
//   - params: Current simulation parameters
//   - partitionIndex: Index of the current partition
//   - stateHistories: All partition state histories
//   - timestepsHistory: Time and timestep information
//
// And returns a slice of float64 values representing the computed output.
//
// Applications:
//   - Feature engineering: Compute derived features from raw simulation data
//   - Data transformation: Apply mathematical transformations to state values
//   - Custom aggregations: Implement specialized aggregation logic
//   - Parameter synthesis: Combine multiple parameters into derived values
//   - Event detection: Compute indicators for significant events
//
// Example:
//
//	iteration := &ValuesFunctionIteration{
//	    Function: func(params *simulator.Params, partitionIndex int,
//	                   stateHistories []*simulator.StateHistory,
//	                   timestepsHistory *simulator.CumulativeTimestepsHistory) []float64 {
//	        // Extract current state from first partition
//	        currentState := stateHistories[0].Values.RawRowView(0)
//
//	        // Compute derived feature: moving average
//	        if len(currentState) >= 2 {
//	            return []float64{(currentState[0] + currentState[1]) / 2.0}
//	        }
//	        return []float64{0.0}
//	    },
//	}
//
// Performance Considerations:
//   - Function is called once per simulation step
//   - Avoid expensive computations in the function body
//   - Consider caching for repeated calculations
//   - Memory allocations should be minimized
//
// API Stability:
//   - This interface is stable and will not change in future versions
//   - Function signature is compatible across all stochadex versions
//
// Related Types:
//   - See NewTransformReduceFunction for composed transform-reduce operations
//   - See ParamsTransform for parameter extraction utilities
//   - See SumReduce for simple reduction operations
type ValuesFunctionIteration struct {
	Function func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []float64
}

func (v *ValuesFunctionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesFunctionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return v.Function(params, partitionIndex, stateHistories, timestepsHistory)
}
