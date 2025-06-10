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

// OtherPartitionPushAndSortFunction
func OtherPartitionPushAndSortFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (SortingValues, bool) {
	return SortingValues{}, false
}

// ParamValuesPushAndSortFunction
func ParamValuesPushAndSortFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (SortingValues, bool) {
	return SortingValues{}, false
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
					outputValues[firstStateValueIndex+j] = sorting.Values[j]
				}
				break
			}
		}
	}
	return outputValues
}
