package general

import (
	"fmt"
	"strconv"

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
	for group, values := range groupings {
		index, ok := outputIndexByGroup[group]
		if !ok {
			continue
		}
		defaultValues[index] = float64(len(values))
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
		if weightings != nil {
			defaultValues[index] = floats.Dot(weightings[group], values)
		} else {
			defaultValues[index] = floats.Sum(values)
		}
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
		if weightings != nil {
			w := weightings[group]
			defaultValues[index] = floats.Dot(w, values) / floats.Sum(w)
		} else {
			defaultValues[index] = floats.Sum(values) / float64(len(values))
		}
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
		if weightings != nil {
			floats.Mul(values, weightings[group])
			defaultValues[index] = floats.Max(values)
		} else {
			defaultValues[index] = floats.Max(values)
		}
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
		if weightings != nil {
			floats.Mul(values, weightings[group])
			defaultValues[index] = floats.Min(values)
		} else {
			defaultValues[index] = floats.Min(values)
		}
	}
	return defaultValues
}

// RoundToPrecision rounds floats to n decimal places.
func RoundToPrecision(value float64, precision int) float64 {
	format := "%." + strconv.Itoa(precision) + "f"
	roundedValue, _ := strconv.ParseFloat(fmt.Sprintf(format, value), 64)
	return roundedValue
}

// FloatTupleToKey converts a slice of floats to a string key with
// fixed precision for float values.
func FloatTupleToKey(tuple []float64, precision int) string {
	key := ""
	for _, v := range tuple {
		rounded := RoundToPrecision(v, precision)
		key += strconv.FormatFloat(rounded, 'f', precision, 64) + ","
	}
	return key
}

// ValuesGroupedAggregationIteration defines an iteration which applies
// a user-defined aggregation function to the last input values from
// other partitions and groups them into bins.
type ValuesGroupedAggregationIteration struct {
	Aggregation func(
		defaultValues []float64,
		outputIndexByGroup map[string]int,
		groupings map[string][]float64,
		weightings map[string][]float64,
	) []float64
	outputIndexByGroup map[string]int
	tupleLength        int
	precision          int
}

func (v *ValuesGroupedAggregationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.outputIndexByGroup = make(map[string]int)
	var valueGroupTuples [][]float64
	v.tupleLength = 0
	for {
		groupValues, ok := settings.Params[partitionIndex].GetOk(
			"accepted_value_group_index_" + strconv.Itoa(v.tupleLength))
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
	v.precision = int(settings.Params[partitionIndex].Get(
		"float_precision")[0])
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
	var weightings map[string][]float64
	if _, ok := params.GetOk("weightings"); ok {
		weightings = make(map[string][]float64)
	}
	groupings := make(map[string][]float64)
	var values []float64
	var ok bool
	for i, statePartitionIndex := range params.Get("state_partitions") {
		tuple := make([]float64, 0)
		for j := 0; j < v.tupleLength; j++ {
			tupleIndex := strconv.Itoa(j)
			tuple = append(tuple, stateHistories[int(
				params.GetIndex("grouping_partitions_index_"+
					tupleIndex, i))].Values.At(0, int(params.GetIndex(
				"grouping_value_indices_index_"+tupleIndex, i)),
			))
		}
		groupKey := FloatTupleToKey(tuple, v.precision)
		stateValue := stateHistories[int(statePartitionIndex)].Values.At(
			0, int(params.GetIndex("state_value_indices", i)),
		)
		values, ok = groupings[groupKey]
		if !ok {
			groupings[groupKey] = []float64{stateValue}
			continue
		}
		values = append(values, stateValue)
		groupings[groupKey] = values
	}
	return v.Aggregation(
		aggValues,
		v.outputIndexByGroup,
		groupings,
		weightings,
	)
}
