package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SortingValues encapsulates a new entry to add to the sorting collection.
type SortingValues struct {
	SortBy float64
	Values []float64
}

// OtherPartitionsPushAndSortFunction retrieves values from one partition and
// sorts by another partition's value.
//
// Usage hints:
//   - Provide: "other_partition", "value_indices", "other_partition_sort_by",
//     and "value_index_sort_by". Skip push if first value equals "empty_value".
func OtherPartitionsPushAndSortFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (SortingValues, bool) {
	nextValues := make([]float64, 0)
	stateHistory := stateHistories[int(params.GetIndex("other_partition", 0))]
	for _, index := range params.Get("value_indices") {
		nextValues = append(nextValues, stateHistory.Values.At(0, int(index)))
		if index == 0 && params.GetIndex("empty_value", 0) == nextValues[0] {
			return SortingValues{}, false
		}
	}
	return SortingValues{
		SortBy: stateHistories[int(
			params.GetIndex("other_partition_sort_by", 0))].Values.At(0, int(
			params.GetIndex("value_index_sort_by", 0))),
		Values: nextValues,
	}, true
}

// ParamValuesPushAndSortFunction uses params for both values and sort key.
//
// Usage hints:
//   - Provide: "next_values_push", "empty_value", and "next_values_sort_by".
func ParamValuesPushAndSortFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (SortingValues, bool) {
	nextValues := params.Get("next_values_push")
	if params.GetIndex("empty_value", 0) == nextValues[0] {
		return SortingValues{}, false
	}
	return SortingValues{
		SortBy: params.GetIndex("next_values_sort_by", 0),
		Values: nextValues,
	}, true
}

// ValuesSortingCollectionIteration maintains a sorted collection of entries.
//
// Usage hints:
//   - Provide: "values_state_width" (entry width minus 1 for sort key) and
//     "empty_value" sentinel. Set PushAndSort to define insertion behaviour.
type ValuesSortingCollectionIteration struct {
	PushAndSort func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) (SortingValues, bool)
}

func (v *ValuesSortingCollectionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesSortingCollectionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	outputValues := stateHistory.GetNextStateRowToUpdate()
	emptyValue := params.GetIndex("empty_value", 0)
	entryWidth := int(params.GetIndex("values_state_width", 0)) + 1
	if sorting, push := v.PushAndSort(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	); push {
		for i := range int(stateHistory.StateWidth / entryWidth) {
			firstStateValueIndex := i * entryWidth
			sortByKey := outputValues[firstStateValueIndex]
			insertAndShift := sorting.SortBy > sortByKey
			if sortByKey == emptyValue || insertAndShift {
				if insertAndShift {
					for k := firstStateValueIndex +
						entryWidth; k < len(outputValues); k++ {
						outputValues[k] = outputValues[k-entryWidth]
					}
				}
				for j := range entryWidth {
					if j == 0 {
						outputValues[firstStateValueIndex] = sorting.SortBy
						continue
					}
					outputValues[firstStateValueIndex+j] = sorting.Values[j-1]
				}
				break
			}
		}
	}
	return outputValues
}
