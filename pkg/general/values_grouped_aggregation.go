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

// GroupStateValue represents a grouping value and state value pair.
type GroupStateValue struct {
	Group float64
	State float64
}

// GroupStateParamValuesFunction generates group values and state values
// based directly on input params from the user.
func GroupStateParamValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []GroupStateValue {
	values := make([]GroupStateValue, 0)
	groupValues := params.Get("group_values")
	for i, stateValue := range params.Get("state_values") {
		values = append(
			values,
			GroupStateValue{
				Group: groupValues[i],
				State: stateValue,
			},
		)
	}
	return values
}

// ZeroGroupStateParamValuesFunction generates state values based
// directly on input params from the user. In addition, a group value
// of 0.0 is always used.
func ZeroGroupStateParamValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []GroupStateValue {
	values := make([]GroupStateValue, 0)
	for _, stateValue := range params.Get("state_values") {
		values = append(
			values,
			GroupStateValue{
				Group: 0.0,
				State: stateValue,
			},
		)
	}
	return values
}

// PartitionRangesValuesFunction generates group values and state values
// based on ranges of partition and value indices which are used to
// retrieve the latest corresponding data from the state history of the
// specified partitions.
func PartitionRangesValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []GroupStateValue {
	values := make([]GroupStateValue, 0)
	for i, statePartitionIndex := range params.Get("state_partition_indices") {
		values = append(
			values,
			GroupStateValue{
				Group: stateHistories[int(
					params.GetIndex("grouping_partition_indices", i))].Values.At(
					0,
					int(params.GetIndex("grouping_value_indices", i)),
				),
				State: stateHistories[int(statePartitionIndex)].Values.At(
					0,
					int(params.GetIndex("state_value_indices", i)),
				),
			},
		)
	}
	return values
}

// ZeroGroupPartitionRangesValuesFunction generates state values
// based on ranges of partition and value indices which are used to
// retrieve the latest corresponding data from the state history of the
// specified partitions. In addition, a group value of 0.0 is always used.
func ZeroGroupPartitionRangesValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []GroupStateValue {
	values := make([]GroupStateValue, 0)
	for i, statePartitionIndex := range params.Get("state_partition_indices") {
		values = append(
			values,
			GroupStateValue{
				Group: 0.0,
				State: stateHistories[int(statePartitionIndex)].Values.At(
					0,
					int(params.GetIndex("state_value_indices", i)),
				),
			},
		)
	}
	return values
}

// ValuesGroupedAggregationIteration defines an iteration which applies
// a user-defined aggregation function to the last input values from
// other partitions and groups them into bins specified by the
// "value_groups" params.
type ValuesGroupedAggregationIteration struct {
	ValuesFunction func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []GroupStateValue
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
	for i, value := range settings.Params[partitionIndex].Get("accepted_value_groups") {
		v.outputIndexByValueGroup[value] = i
	}
}

func (v *ValuesGroupedAggregationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	countByValueGroup := make(map[float64]int)
	aggValues := make([]float64, stateHistories[partitionIndex].StateWidth)
	if defaultValues, ok := params.GetOk("default_values"); ok {
		for i, value := range defaultValues {
			aggValues[i] = value
		}
	}
	for _, groupStateValue := range v.ValuesFunction(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	) {
		if outputIndex, ok := v.outputIndexByValueGroup[groupStateValue.Group]; ok {
			count, ok := countByValueGroup[groupStateValue.Group]
			if !ok {
				countByValueGroup[groupStateValue.Group] = 0
				count = 0
			}
			count += 1
			countByValueGroup[groupStateValue.Group] = count
			aggValues[outputIndex] = v.AggFunction(
				aggValues[outputIndex],
				count,
				groupStateValue.State,
			)
		}
	}
	return aggValues
}
