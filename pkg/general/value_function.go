package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValueFunctionIteration defines an iteration which wraps a
// user-specified function. This iteration is fully stateless.
type ValueFunctionIteration struct {
	Function func(
		params simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []float64
}

func (v *ValueFunctionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValueFunctionIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return v.Function(params, partitionIndex, stateHistories, timestepsHistory)
}
