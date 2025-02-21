package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// FromHistoryIteration provides a stream of data which is already known from a
// separate data source and is held in memory as a simulator.StateHistory.
type FromHistoryIteration struct {
	Data *simulator.StateHistory
}

func (f *FromHistoryIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (f *FromHistoryIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	var data []float64
	// starts from one step into the window because it makes it possible to
	// use the i := f.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := f.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber - 1; i >= 0 {
		data = f.Data.Values.RawRowView(i)
	} else if i == -1 {
		data = params.Get("latest_data_values")
	} else {
		panic("timesteps have gone beyond the available data")
	}
	return data
}

func (f *FromHistoryIteration) UpdateMemory(
	params *simulator.Params,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	f.Data = stateHistories[int(params.GetIndex("data_partition", 0))]
}

// FromHistoryTimestepFunction provides a stream of timesteps which already known from
// a separate data source and is held in memory as a simulator.CumulativeTimestepsHistory.
type FromHistoryTimestepFunction struct {
	Data *simulator.CumulativeTimestepsHistory
}

func (f *FromHistoryTimestepFunction) NextIncrement(
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) float64 {
	// starts from one step into the window because it makes it possible to
	// use the i := f.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := f.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber - 1; i >= 0 {
		return f.Data.Values.AtVec(i) - timestepsHistory.Values.AtVec(0)
	} else if i == -1 {
		return f.Data.NextIncrement
	} else {
		panic("timesteps have gone beyond the available data")
	}
}
