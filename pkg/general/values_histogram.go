package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValuesHistogramIteration defines an iteration which bins the last
// input values from other partitions into table of value frequencies.
type ValuesHistogramIteration struct {
	outputIndexByValue map[float64]int
}

func (v *ValuesHistogramIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.outputIndexByValue = make(map[float64]int)
	for i, value := range settings.Params[partitionIndex]["values_to_count"] {
		v.outputIndexByValue[value] = i
	}
}

func (v *ValuesHistogramIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if indices, ok := params["partition_indices"]; ok {
		stateValues := make([]float64, len(indices))
		for i, index := range indices {
			stateValues[i] = stateHistories[int(index)].Values.At(
				0,
				int(params["state_value_indices"][i]),
			)
		}
		params["state_values"] = stateValues
	}
	histogramValues := make([]float64, stateHistories[partitionIndex].StateWidth)
	for _, stateValue := range params["state_values"] {
		histogramValues[v.outputIndexByValue[stateValue]] += 1
	}
	return histogramValues
}
