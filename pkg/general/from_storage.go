package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// FromStorageIteration streams data from an in-memory [][]float64.
//
// Usage hints:
//   - Provide Data (rows over time); supports initial offset via InitStepsTaken.
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

// FromStorageTimestepFunction streams timesteps from an in-memory []float64.
//
// Usage hints:
//   - Provide Data; supports initial offset via InitStepsTaken.
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
