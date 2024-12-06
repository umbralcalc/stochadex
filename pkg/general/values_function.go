package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// ParamsTransform simply returns the params.
func ParamsTransform(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) map[string][]float64 {
	return params.Map
}

// SumReduce computes the sum reduction.
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

// NewTransformReduceFunction creates a new function that applies
// the provided transformation and reduction function operations
// as a composition that can be used in the ValuesFunctionIteration.
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

// ValuesFunctionIteration defines an iteration which wraps a
// user-specified function. This iteration is fully stateless.
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
