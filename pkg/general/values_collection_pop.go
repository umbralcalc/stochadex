package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NextNonEmptyPopIndexFunction returns the index of the next non-
// empty value found in the collection.
func NextNonEmptyPopIndexFunction(
	params simulator.Params,
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

// ValuesCollectionPopIteration maintains a collection of same-size
// state values and pops an indexed value set from this collection
// depending on the output of a user-specified function.
type ValuesCollectionPopIteration struct {
	PopIndexFunction func(
		params simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) (int, bool)
}

func (v *ValuesCollectionPopIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesCollectionPopIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := stateHistories[partitionIndex].Values.RawRowView(0)
	emptyValue := params.GetIndex("empty_value", 0)
	valuesWidth := int(params.GetIndex("values_state_width", 0))
	// clear the last popped values if they exist
	for i := 0; i < valuesWidth; i++ {
		outputValues[i] = emptyValue
	}
	if index, pop := v.PopIndexFunction(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	); pop {
		// values at index 0 of the collection are used to indicate
		// a value pop by convention
		firstStateValueIndex := index * valuesWidth
		for j := 0; j < valuesWidth; j++ {
			outputValues[j] = outputValues[firstStateValueIndex+j]
			outputValues[firstStateValueIndex+j] = emptyValue
		}
	}
	return outputValues
}
