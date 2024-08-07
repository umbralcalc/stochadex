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
	for _, index := range params["connected_partitions"] {
		for _, valueIndex := range params["connected_value_indices"] {
			histogramValues[int(stateHistories[int(index)].Values.At(0, int(valueIndex)))] += 1
		}
	}
	return histogramValues
}
