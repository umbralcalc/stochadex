package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParamValuesIteration writes the float param values under "param_values"
// directly to the state.
//
// Usage hints:
//   - Useful for injecting immediate parameter-driven values.
type ParamValuesIteration struct {
}

func (p *ParamValuesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *ParamValuesIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return params.Get("param_values")
}
