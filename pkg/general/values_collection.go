package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NextNonEmptyPopIndexFunction scans the rolling collection for the next
// occupied slot (index > 0) and returns its index for popping.
//
// Usage hints:
//   - Index 0 is reserved for the most recently popped values.
//   - Configure "values_state_width" and "empty_value" to mark empty slots.
func NextNonEmptyPopIndexFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) (int, bool) {
	emptyValue := params.GetIndex("empty_value", 0)
	valuesWidth := int(params.GetIndex("values_state_width", 0))
	latestValues := stateHistories[partitionIndex].Values.RawRowView(0)
	for i := 1; i < int(
		stateHistories[partitionIndex].StateWidth/valuesWidth); i++ {
		if latestValues[i*valuesWidth] != emptyValue {
			return i, true
		}
	}
	return 0, false
}

// OtherPartitionPushFunction collects the latest values from another
// partition to push into the collection, subject to empty_value.
//
// Usage hints:
//   - Provide: "other_partition" and "value_indices".
//   - If the first element equals "empty_value", the push is skipped.
func OtherPartitionPushFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) ([]float64, bool) {
	nextValues := make([]float64, 0)
	stateHistory := stateHistories[int(params.GetIndex("other_partition", 0))]
	for _, index := range params.Get("value_indices") {
		nextValues = append(nextValues, stateHistory.Values.At(0, int(index)))
		if index == 0 && params.GetIndex("empty_value", 0) == nextValues[0] {
			return nil, false
		}
	}
	return nextValues, true
}

// PopFromOtherCollectionPushFunction pulls the current popped values from
// another collection-like partition and uses them as the next values to push.
//
// Usage hints:
//   - Provide: "other_partition" and "values_state_width".
//   - If the first element equals "empty_value", the push is skipped.
func PopFromOtherCollectionPushFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) ([]float64, bool) {
	nextValues := make([]float64, 0)
	stateHistory := stateHistories[int(params.GetIndex("other_partition", 0))]
	for index := range int(params.GetIndex("values_state_width", 0)) {
		nextValues = append(nextValues, stateHistory.Values.At(0, int(index)))
		if index == 0 && params.GetIndex("empty_value", 0) == nextValues[0] {
			return nil, false
		}
	}
	return nextValues, true
}

// ParamValuesPushFunction reads the next values directly from params under
// "next_values_push", subject to empty_value.
//
// Usage hints:
//   - Provide: "next_values_push" and "empty_value".
func ParamValuesPushFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) ([]float64, bool) {
	nextValues := params.Get("next_values_push")
	if params.GetIndex("empty_value", 0) == nextValues[0] {
		return nil, false
	}
	return nextValues, true
}

// ValuesCollectionIteration maintains a fixed-width rolling collection of
// value vectors.
//
// Usage hints:
//   - Provide: "values_state_width" and "empty_value" for sentinel handling.
//   - Set Push to define how new values are appended; set PopIndex to surface
//     an existing entry into index 0 and clear that slot.
type ValuesCollectionIteration struct {
	PopIndex func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) (int, bool)
	Push func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) ([]float64, bool)
}

func (v *ValuesCollectionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesCollectionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	outputValues := stateHistory.GetNextStateRowToUpdate()
	emptyValue := params.GetIndex("empty_value", 0)
	valuesWidth := int(params.GetIndex("values_state_width", 0))
	if v.Push != nil {
		if values, push := v.Push(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		); push {
			collectionFull := true
			// values at index 0 of the collection are ignored by convention
			// since they are used to indicate a value pop
			for i := 1; i < int(stateHistory.StateWidth/valuesWidth); i++ {
				firstStateValueIndex := i * valuesWidth
				if outputValues[firstStateValueIndex] == emptyValue {
					for j := range valuesWidth {
						outputValues[firstStateValueIndex+j] = values[j]
					}
					collectionFull = false
					break
				}
			}
			if collectionFull {
				panic("values collection has been completely filled")
			}
		}
	}
	if v.PopIndex != nil {
		// clear the last popped values if they exist
		for i := range valuesWidth {
			outputValues[i] = emptyValue
		}
		if index, pop := v.PopIndex(
			params,
			partitionIndex,
			stateHistories,
			timestepsHistory,
		); pop {
			// values at index 0 of the collection are used to indicate
			// a value pop by convention
			firstStateValueIndex := index * valuesWidth
			for j := range valuesWidth {
				outputValues[j] = outputValues[firstStateValueIndex+j]
				outputValues[firstStateValueIndex+j] = emptyValue
			}
		}
	}
	return outputValues
}
