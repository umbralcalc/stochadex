package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// FromStorageIteration provides a stream of data which is already known from a
// separate data source and is held in memory as a [][]float64.
type FromStorageIteration struct {
	Data           [][]float64
	InitStepsTaken int
}

func (f *FromStorageIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (f *FromStorageIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	var data []float64
	// starts from one step into the data because it makes it possible to
	// use the i := 0 value for the initial conditions
	if i := timestepsHistory.CurrentStepNumber +
		f.InitStepsTaken; i < len(f.Data) {
		data = f.Data[i]
	} else {
		panic("timesteps have gone beyond the available data")
	}
	return data
}

// FromStorageTimestepFunction provides a stream of timesteps which already
// known from a separate data source and is held in memory as a []float64.
type FromStorageTimestepFunction struct {
	Data           []float64
	InitStepsTaken int
}

func (f *FromStorageTimestepFunction) NextIncrement(
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) float64 {
	// starts from one step into the data because it makes it possible to
	// use the i := 0 value for the initial conditions
	if i := timestepsHistory.CurrentStepNumber +
		f.InitStepsTaken; i < len(f.Data) {
		return f.Data[i] - timestepsHistory.Values.AtVec(0)
	} else {
		panic("timesteps have gone beyond the available data")
	}
}
