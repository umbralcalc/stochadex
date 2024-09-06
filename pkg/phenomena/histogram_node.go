package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// HistogramNodeIteration collects the frequencies of states being occupied
// by all of the specified connected partitions over the latest step in the state history.
type HistogramNodeIteration struct {
}

func (h *HistogramNodeIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (h *HistogramNodeIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	histogramValues := stateHistories[partitionIndex].Values.RawRowView(0)
	for i, index := range params["connected_partitions"] {
		state := int(stateHistories[int(index)].Values.At(
			0,
			int(params["connected_state_value_indices"][i]),
		))
		histogramValues[state] += 1
	}
	return histogramValues
}
