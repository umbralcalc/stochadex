package params

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ParamsReaderIteration wraps any iteration and gives it the functionality to
// read its masked float and int params from the last values in the state
// history of another partition.
type ParamsReaderIteration struct {
	Iteration       simulator.Iteration
	paramsMappings  *ParamsMappings
	partitionToRead int
}

func (p *ParamsReaderIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Iteration.Configure(partitionIndex, settings)
	p.paramsMappings = NewParamsMappings(settings.OtherParams[partitionIndex])
	p.partitionToRead = int(settings.OtherParams[partitionIndex].
		IntParams["partition_to_read"][0])
}

func (p *ParamsReaderIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return p.Iteration.Iterate(
		p.paramsMappings.UpdateParamsFromFlattened(
			stateHistories[p.partitionToRead].Values.RawRowView(0),
			params,
		),
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
}
