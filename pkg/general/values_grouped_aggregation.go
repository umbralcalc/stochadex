package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CountAggFunction returns the count of values in the group.
func CountAggFunction(
	currentAggValue float64,
	nextValueCount int,
	nextValue float64,
) float64 {
	return float64(nextValueCount)
}

// SumAggFunction returns the sum of values in the group.
func SumAggFunction(
	currentAggValue float64,
	nextValueCount int,
	nextValue float64,
) float64 {
	return currentAggValue + nextValue
}

// MeanAggFunction returns the mean of values in the group.
func MeanAggFunction(
	currentAggValue float64,
	nextValueCount int,
	nextValue float64,
) float64 {
	return currentAggValue + ((nextValue - currentAggValue) / float64(nextValueCount))
}

// MaxAggFunction returns the maximum of values in the group.
func MaxAggFunction(
	currentAggValue float64,
	nextValueCount int,
	nextValue float64,
) float64 {
	if currentAggValue < nextValue {
		return nextValue
	} else {
		return currentAggValue
	}
}

// MinAggFunction returns the minimum of values in the group.
func MinAggFunction(
	currentAggValue float64,
	nextValueCount int,
	nextValue float64,
) float64 {
	if currentAggValue > nextValue {
		return nextValue
	} else {
		return currentAggValue
	}
}

// ValuesGroupedAggregationIteration defines an iteration which applies
// a user-defined aggregation function to the last input values from
// other partitions and groups them into bins specified by the
// "value_groups" params.
type ValuesGroupedAggregationIteration struct {
	AggFunction func(
		currentAggValue float64,
		nextValueCount int,
		nextValue float64,
	) float64
	outputIndexByValueGroup map[float64]int
}

func (v *ValuesGroupedAggregationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.outputIndexByValueGroup = make(map[float64]int)
	for i, value := range settings.Params[partitionIndex]["value_groups"] {
		v.outputIndexByValueGroup[value] = i
	}
}

func (v *ValuesGroupedAggregationIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	countByValueGroup := make(map[float64]int)
	aggValues := make([]float64, stateHistories[partitionIndex].StateWidth)
	for i, statePartitionIndex := range params["state_partition_indices"] {
		stateValue := stateHistories[int(statePartitionIndex)].Values.At(
			0,
			int(params["state_value_indices"][i]),
		)
		groupingValue := stateHistories[int(
			params["grouping_partition_indices"][i])].Values.At(
			0,
			int(params["grouping_value_indices"][i]),
		)
		if outputIndex, ok := v.outputIndexByValueGroup[groupingValue]; ok {
			count, ok := countByValueGroup[groupingValue]
			if !ok {
				countByValueGroup[groupingValue] = 0
				count = 0
			}
			count += 1
			countByValueGroup[groupingValue] = count
			aggValues[outputIndex] = v.AggFunction(
				aggValues[outputIndex],
				count,
				stateValue,
			)
		}
	}
	return aggValues
}
