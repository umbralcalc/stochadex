package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

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
	for index := 0; index < int(params.GetIndex("values_state_width", 0)); index++ {
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

// ValuesCollectionPushIteration maintains a collection of same-size
// state values and pushes more to the collection depending on the
// output of a user-specified function.
type ValuesCollectionPushIteration struct {
	PushFunction func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) ([]float64, bool)
}

func (v *ValuesCollectionPushIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesCollectionPushIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	outputValues := stateHistory.Values.RawRowView(0)
	if values, push := v.PushFunction(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	); push {
		emptyValue := params.GetIndex("empty_value", 0)
		valuesWidth := int(params.GetIndex("values_state_width", 0))
		collectionFull := true
		// values at index 0 of the collection are ignored by convention
		// since they are used to indicate a value pop
		for i := 1; i < int(stateHistory.StateWidth/valuesWidth); i++ {
			firstStateValueIndex := i * valuesWidth
			if outputValues[firstStateValueIndex] == emptyValue {
				for j := 0; j < valuesWidth; j++ {
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
	return outputValues
}
