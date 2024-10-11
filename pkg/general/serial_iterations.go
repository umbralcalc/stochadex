package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// SerialIterationsIteration applies a slice of iterations serially,
// updating the latest value this partition's state history for each
// iteration with the output from the previous iteration in the slice.
type SerialIterationsIteration struct {
	Iterations []simulator.Iteration
}

func (s *SerialIterationsIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	for _, iteration := range s.Iterations {
		iteration.Configure(partitionIndex, settings)
	}
}

func (s *SerialIterationsIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistoriesClone := make([]*simulator.StateHistory, len(stateHistories))
	stateHistory := stateHistories[partitionIndex]
	stateHistoryClone := simulator.StateHistory{
		Values:            mat.DenseCopyOf(stateHistory.Values),
		NextValues:        stateHistory.NextValues,
		StateWidth:        stateHistory.StateWidth,
		StateHistoryDepth: stateHistory.StateHistoryDepth,
	}
	for i, stateHistory := range stateHistories {
		switch i {
		case partitionIndex:
			stateHistoriesClone[i] = &stateHistoryClone
		default:
			stateHistoriesClone[i] = stateHistory
		}
	}
	for _, iteration := range s.Iterations {
		stateHistoryClone.Values.SetRow(0, iteration.Iterate(
			params,
			partitionIndex,
			stateHistoriesClone,
			timestepsHistory,
		))
	}
	return stateHistoryClone.Values.RawRowView(0)
}
