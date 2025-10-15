package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// FromHistoryIteration streams data from an in-memory StateHistory.
//
// Usage hints:
//   - Use with EmbeddedSimulationRun or analysis windows to replay data.
//   - Set Data to the source history; supports initial offset via InitStepsTaken.
type FromHistoryIteration struct {
	Data           *simulator.StateHistory
	InitStepsTaken int
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
	if i := f.Data.StateHistoryDepth -
		timestepsHistory.CurrentStepNumber - (f.InitStepsTaken + 1); i >= 0 {
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
	update StateMemoryUpdate,
) {
	f.Data = update.StateHistory
}

// FromHistoryTimestepFunction streams timesteps from an in-memory
// CumulativeTimestepsHistory.
//
// Usage hints:
//   - Set Data to the source timestep series; supports initial offset via InitStepsTaken.
type FromHistoryTimestepFunction struct {
	Data           *simulator.CumulativeTimestepsHistory
	InitStepsTaken int
}

func (f *FromHistoryTimestepFunction) NextIncrement(
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) float64 {
	// starts from one step into the window because it makes it possible to
	// use the i := f.Data.StateHistoryDepth - timestepsHistory.CurrentStepNumber value
	// for the initial conditions
	if i := f.Data.StateHistoryDepth -
		timestepsHistory.CurrentStepNumber - (f.InitStepsTaken + 1); i >= 0 {
		return f.Data.Values.AtVec(i) - timestepsHistory.Values.AtVec(0)
	} else if i == -1 {
		return f.Data.NextIncrement
	} else {
		panic("timesteps have gone beyond the available data")
	}
}
