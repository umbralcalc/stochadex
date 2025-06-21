package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SortingValues specifies a new set of values to be added to the
// sorting collection.
type SortingValues struct {
	SortBy float64
	Values []float64
}

// OtherPartitionsPushAndSortFunction retrieves the next values to
// push from the last values of another partition and sorts by the values
// of yet another partition. In the former case, if the first value is
// equal to the "empty_value" param then nothing is pushed.
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

// ParamValuesPushAndSortFunction retrieves the next values to
// push from the "next_values_push" params and if the first value is equal
// to the "empty_value" param then nothing is pushed. It also sorts by
// the "next_values_sort_by" param.
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

// ValuesSortingCollectionIteration maintains a sorted collection
// of same-size state values. You can push more to the collection
// depending on the output of a user-specified function, where values
// are removed when they are sorted out of the full collection size.
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
