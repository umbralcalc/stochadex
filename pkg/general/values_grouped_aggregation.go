package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// CountAggregation returns the count of values in the group.
func CountAggregation(
	groupings Groupings,
	outputIndexByGroup map[float64]int,
	output []float64,
) {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		output[index] = float64(len(values))
	}
}

// SumAggregation returns the sum of values in the group.
func SumAggregation(
	groupings Groupings,
	outputIndexByGroup map[float64]int,
	output []float64,
) {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		output[index] = floats.Sum(values)
	}
}

// MeanAggregation returns the mean of values in the group.
func MeanAggregation(
	groupings Groupings,
	outputIndexByGroup map[float64]int,
	output []float64,
) {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		output[index] = floats.Sum(values) / float64(len(values))
	}
}

// MaxAggregation returns the maximum of values in the group.
func MaxAggregation(
	groupings Groupings,
	outputIndexByGroup map[float64]int,
	output []float64,
) {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		output[index] = floats.Max(values)
	}
}

// MinAggregation returns the minimum of values in the group.
func MinAggregation(
	groupings Groupings,
	outputIndexByGroup map[float64]int,
	output []float64,
) {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		output[index] = floats.Min(values)
	}
}

// Groupings represents the groups of values for grouped aggregations.
type Groupings map[float64][]float64

// ParamValuesGrouping generates group values and state values based
// directly on input params from the user.
func ParamValuesGrouping(
	params *simulator.Params,
	stateHistories []*simulator.StateHistory,
) Groupings {
	groupings := make(Groupings)
	groupValues := params.Get("group_values")
	var values []float64
	var ok bool
	for i, stateValue := range params.Get("state_values") {
		values, ok = groupings[groupValues[i]]
		if !ok {
			groupings[groupValues[i]] = []float64{stateValue}
			continue
		}
		values = append(values, stateValue)
		groupings[groupValues[i]] = values
	}
	return groupings
}

// PartitionRangesGrouping generates group values and state values based
// on ranges of partition and value indices which are used to retrieve
// the latest corresponding data from the state history of the specified
// partitions.
func PartitionRangesGrouping(
	params *simulator.Params,
	stateHistories []*simulator.StateHistory,
) Groupings {
	groupings := make(Groupings)
	var values []float64
	var ok bool
	for i, statePartitionIndex := range params.Get("state_partitions") {
		groupValue := stateHistories[int(
			params.GetIndex("grouping_partitions", i))].Values.At(
			0, int(params.GetIndex("grouping_value_indices", i)),
		)
		stateValue := stateHistories[int(statePartitionIndex)].Values.At(
			0, int(params.GetIndex("state_value_indices", i)),
		)
		values, ok = groupings[groupValue]
		if !ok {
			groupings[groupValue] = []float64{stateValue}
			continue
		}
		values = append(values, stateValue)
		groupings[groupValue] = values
	}
	return groupings
}

// ValuesGroupedAggregationIteration defines an iteration which applies
// a user-defined aggregation function to the last input values from
// other partitions and groups them into bins specified by the
// "value_groups" params.
type ValuesGroupedAggregationIteration struct {
	Grouping func(
		params *simulator.Params,
		stateHistories []*simulator.StateHistory,
	) Groupings
	Aggregation func(
		groupings Groupings,
		outputIndexByGroup map[float64]int,
		output []float64,
	)
	outputIndexByGroup map[float64]int
}

func (v *ValuesGroupedAggregationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.outputIndexByGroup = make(map[float64]int)
	for i, value := range settings.Params[partitionIndex].Get(
		"accepted_value_groups") {
		v.outputIndexByGroup[value] = i
	}
}

func (v *ValuesGroupedAggregationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	aggValues := make([]float64, stateHistories[partitionIndex].StateWidth)
	if defaultValues, ok := params.GetOk("default_values"); ok {
		copy(aggValues, defaultValues)
	}
	v.Aggregation(
		v.Grouping(params, stateHistories),
		v.outputIndexByGroup,
		aggValues,
	)
	return aggValues
}
