package params

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParamsReaderIteration wraps any iteration and gives it the functionality to
// read its masked float and int params from the next values in the state
// history of another serially-upstream partition.
type ParamsReaderIteration struct {
	Iteration       simulator.Iteration
	paramsMappings  *ParamsMappings
	paramsPartition int
}

func (p *ParamsReaderIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Iteration.Configure(partitionIndex, settings)
	p.paramsMappings = NewParamsMappings(settings.OtherParams[partitionIndex])
	p.paramsPartition = int(settings.OtherParams[partitionIndex].IntParams["params_partition"][0])
}

func (p *ParamsReaderIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return p.Iteration.Iterate(
		p.paramsMappings.UpdateParamsFromFlattened(
			stateHistories[p.paramsPartition].NextValues,
			params,
		),
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
}
