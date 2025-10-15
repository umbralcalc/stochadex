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

// ValuesFunctionIteration evaluates a user-provided function of params,
// states and time each step.
//
// Usage hints:
//   - Set Function to a pure mapping from the simulation context to values.
//   - Useful for feature engineering inside the simulator.
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
