package params

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParamsReaderIteration wraps any iteration and gives it the functionality to
// read its masked float and int params from the next values in the state
// history of another typically-upstream partition.
type ParamsReaderIteration struct {
	Iteration      simulator.Iteration
	paramsMappings *ParamsMappings
}

func (p *ParamsReaderIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Iteration.Configure(partitionIndex, settings)
	p.paramsMappings = NewParamsMappings(settings.OtherParams[partitionIndex])
}

func (p *ParamsReaderIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return p.Iteration.Iterate(
		p.paramsMappings.UpdateParamsFromFlattened(
			params.FloatParams["param_values"],
			params,
		),
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
}
