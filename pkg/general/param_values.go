package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParamValuesIteration writes the float param values in the
// "param_values" key directly to the state.
type ParamValuesIteration struct {
}

func (p *ParamValuesIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *ParamValuesIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return params.Get("param_values")
}
