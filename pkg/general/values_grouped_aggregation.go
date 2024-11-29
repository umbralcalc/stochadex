package general

import (
	"fmt"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// CountAggregation returns the count of values in the group.
func CountAggregation(
	defaultValues []float64,
	outputIndexByGroup map[string]int,
	groupings map[string][]float64,
	weightings map[string][]float64,
) []float64 {
	for group, weights := range weightings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		defaultValues[index] = floats.Sum(weights)
	}
	return defaultValues
}

// SumAggregation returns the sum of values in the group.
func SumAggregation(
	defaultValues []float64,
	outputIndexByGroup map[string]int,
	groupings map[string][]float64,
	weightings map[string][]float64,
) []float64 {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		defaultValues[index] = floats.Dot(weightings[group], values)
	}
	return defaultValues
}

// MeanAggregation returns the mean of values in the group.
func MeanAggregation(
	defaultValues []float64,
	outputIndexByGroup map[string]int,
	groupings map[string][]float64,
	weightings map[string][]float64,
) []float64 {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		w := weightings[group]
		defaultValues[index] = floats.Dot(w, values) / floats.Sum(w)
	}
	return defaultValues
}

// MaxAggregation returns the maximum of values in the group.
func MaxAggregation(
	defaultValues []float64,
	outputIndexByGroup map[string]int,
	groupings map[string][]float64,
	weightings map[string][]float64,
) []float64 {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		floats.Mul(values, weightings[group])
		defaultValues[index] = floats.Max(values)
	}
	return defaultValues
}

// MinAggregation returns the minimum of values in the group.
func MinAggregation(
	defaultValues []float64,
	outputIndexByGroup map[string]int,
	groupings map[string][]float64,
	weightings map[string][]float64,
) []float64 {
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		floats.Mul(values, weightings[group])
		defaultValues[index] = floats.Min(values)
	}
	return defaultValues
}

// AppendFloatToKey appends the provided string key with another
// formatted float value up to the required precision.
func AppendFloatToKey(key string, value float64, precision int) string {
	format := "%." + strconv.Itoa(precision) + "f"
	rounded, _ := strconv.ParseFloat(fmt.Sprintf(format, value), 64)
	key += strconv.FormatFloat(rounded, 'f', precision, 64) + ","
	return key
}

// FloatTupleToKey converts a slice of floats to a string key with
// fixed precision for float values.
func FloatTupleToKey(tuple []float64, precision int) string {
	key := ""
	for _, value := range tuple {
		key = AppendFloatToKey(key, value, precision)
	}
	return key
}

// ValuesGroupedAggregationIteration defines an iteration which applies
// a user-defined aggregation function to the histories of values from
// other partitions and groups them into bins.
type ValuesGroupedAggregationIteration struct {
	Aggregation func(
		defaultValues []float64,
		outputIndexByGroup map[string]int,
		groupings map[string][]float64,
		weightings map[string][]float64,
	) []float64
	Kernel             kernels.IntegrationKernel
	outputIndexByGroup map[string]int
	tupleLength        int
	precision          int
}

func (v *ValuesGroupedAggregationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.Kernel.Configure(partitionIndex, settings)
	v.outputIndexByGroup = make(map[string]int)
	var valueGroupTuples [][]float64
	v.tupleLength = 0
	for {
		groupValues, ok := settings.Iterations[partitionIndex].Params.GetOk(
			"accepted_value_group_tupindex_" + strconv.Itoa(v.tupleLength))
		if !ok {
			break
		} else if v.tupleLength == 0 {
			valueGroupTuples = make([][]float64, len(groupValues))
		}
		for i, groupValue := range groupValues {
			valueGroupTuples[i] = append(valueGroupTuples[i], groupValue)
		}
		v.tupleLength += 1
	}
	v.precision = int(settings.Iterations[partitionIndex].Params.GetIndex(
		"float_precision", 0))
	for i, tuple := range valueGroupTuples {
		v.outputIndexByGroup[FloatTupleToKey(tuple, v.precision)] = i
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
	weightings := make(map[string][]float64)
	groupings := make(map[string][]float64)
	var values []float64
	var groupKey string
	var weight float64
	var ok bool
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	for i, index := range params.Get("state_partitions") {
		statePartitionIndex := int(index)
		stateValueIndex := int(params.GetIndex("state_value_indices", i))
		stateHistory := stateHistories[statePartitionIndex]
		latestStateValueSlice := []float64{params.GetIndex("latest_states_partition_"+
			strconv.Itoa(statePartitionIndex), stateValueIndex)}
		weight = v.Kernel.Evaluate(
			latestStateValueSlice,
			latestStateValueSlice,
			latestTime,
			latestTime,
		)
		groupKey = ""
		for k := 0; k < v.tupleLength; k++ {
			groupKey = AppendFloatToKey(groupKey, params.GetIndex(
				"latest_grouping_partition_"+strconv.Itoa(int(params.GetIndex(
					"grouping_partitions_tupindex_"+strconv.Itoa(k), i))), int(params.GetIndex(
					"grouping_value_indices_tupindex_"+strconv.Itoa(k), i))), v.precision)
		}
		if values, ok = groupings[groupKey]; ok {
			groupings[groupKey] = append(values, latestStateValueSlice[0])
			weightings[groupKey] = append(weightings[groupKey], weight)
		} else {
			groupings[groupKey] = latestStateValueSlice
			weightings[groupKey] = []float64{weight}
		}
		for j := 0; j < stateHistory.StateHistoryDepth; j++ {
			groupKey = ""
			for k := 0; k < v.tupleLength; k++ {
				groupKey = AppendFloatToKey(groupKey, stateHistories[int(params.GetIndex(
					"grouping_partitions_tupindex_"+strconv.Itoa(k), i))].Values.At(
					j, int(params.GetIndex("grouping_value_indices_tupindex_"+
						strconv.Itoa(k), i))), v.precision)
			}
			stateValueSlice := []float64{stateHistory.Values.At(j, stateValueIndex)}
			weight = v.Kernel.Evaluate(
				latestStateValueSlice,
				stateValueSlice,
				latestTime,
				timestepsHistory.Values.AtVec(j),
			)
			if values, ok = groupings[groupKey]; ok {
				groupings[groupKey] = append(values, stateValueSlice[0])
				weightings[groupKey] = append(weightings[groupKey], weight)
			} else {
				groupings[groupKey] = stateValueSlice
				weightings[groupKey] = []float64{weight}
			}
		}
	}
	return v.Aggregation(
		aggValues,
		v.outputIndexByGroup,
		groupings,
		weightings,
	)
}
