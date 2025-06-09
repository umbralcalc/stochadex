package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NextNonEmptyPopIndexFunction returns the index of the next non-
// empty value found in the collection.
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

// OtherPartitionPushFunction retrieves the next values to push from
// the last values of another partition. If the first value is equal
// to the "empty_value" param then nothing is pushed.
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

// PopFromOtherCollectionPushFunction retrieves the next values to
// push from the popped values of another partition which is hence
// assumed to also be another value collection. If the first value
// is equal to the "empty_value" param then nothing is pushed.
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

// ParamValuesPushFunction retrieves the next values to push from
// the "next_value_push" params and if the first value is equal
// to the "empty_value" param then nothing is pushed.
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

// ValuesCollectionIteration maintains a collection of same-size
// state values. You can push more to the collection depending on the
// output of a user-specified function or pop an indexed value set
// from this collection depending on the output of another function.
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
	outputValues := make([]float64, stateHistory.StateWidth)
	copy(outputValues, stateHistory.Values.RawRowView(0))
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
