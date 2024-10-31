package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MemoryIteration provides a stream of data which is already know from a
// separate data source and is held in memory.
type MemoryIteration struct {
	Data *simulator.StateHistory
}

func (m *MemoryIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (m *MemoryIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	var data []float64
	// starts from one step into the window because it makes it possible to
	// use the i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber - 1; i >= 0 {
		data = m.Data.Values.RawRowView(i)
	} else if i == -1 {
		data = params.Get("latest_data_values")
	} else {
		panic("timesteps have gone beyond the available data")
	}
	return data
}

// MemoryTimestepFunction provides a stream of timesteps which already known from
// a separate data source and is held in memory.
type MemoryTimestepFunction struct {
	Data *simulator.CumulativeTimestepsHistory
}

func (m *MemoryTimestepFunction) NextIncrement(
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) float64 {
	// starts from one step into the window because it makes it possible to
	// use the i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := m.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber - 1; i >= 0 {
		return m.Data.Values.AtVec(i) - timestepsHistory.Values.AtVec(0)
	} else if i == -1 {
		return m.Data.NextIncrement
	} else {
		panic("timesteps have gone beyond the available data")
	}
}
