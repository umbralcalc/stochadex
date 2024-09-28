package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValuesFunctionIteration defines an iteration which wraps a
// user-specified function. This iteration is fully stateless.
type ValuesFunctionIteration struct {
	Function func(
		params simulator.Params,
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
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return v.Function(params, partitionIndex, stateHistories, timestepsHistory)
}
