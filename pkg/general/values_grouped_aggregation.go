package general

import (
	"fmt"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// CountAggregation outputs, per group, the sum of weights (i.e., a count when
// weights are all 1).
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

// SumAggregation computes the weighted sum of values per group.
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

// MeanAggregation computes the weighted mean of values per group.
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

// MaxAggregation computes the maximum of weighted values per group.
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

// MinAggregation computes the minimum of weighted values per group.
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

// AppendFloatToKey appends a formatted float to a composite grouping key at
// a fixed precision.
func AppendFloatToKey(key string, value float64, precision int) string {
	format := "%." + strconv.Itoa(precision) + "f"
	rounded, _ := strconv.ParseFloat(fmt.Sprintf(format, value), 64)
	key += strconv.FormatFloat(rounded, 'f', precision, 64) + ","
	return key
}

// FloatTupleToKey converts a vector to a composite key string at fixed
// precision, suitable for use as a map key.
func FloatTupleToKey(tuple []float64, precision int) string {
	key := ""
	for _, value := range tuple {
		key = AppendFloatToKey(key, value, precision)
	}
	return key
}

// ValuesGroupedAggregationIteration collects historical values and weights
// into grouping buckets (defined by tupled grouping series) and applies a
// caller-provided aggregation per bucket.
//
// Usage hints:
//   - Configure accepted group tuples via params: "accepted_value_group_tupindex_k".
//   - Provide grouping source partitions/indices via
//     "grouping_partition_tupindex_k" and "grouping_value_indices_tupindex_k".
//   - Provide the state series via "state_partition" and "state_value_indices".
//   - Set Kernel and precision ("float_precision"); optionally set "default_values".
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
	stateHistory := stateHistories[int(params.GetIndex("state_partition", 0))]
	for i, stateValueIndex := range params.Get("state_value_indices") {
		latestStateValueSlice := []float64{
			params.GetIndex("latest_states", int(stateValueIndex))}
		weight = v.Kernel.Evaluate(
			latestStateValueSlice,
			latestStateValueSlice,
			latestTime,
			latestTime,
		)
		groupKey = ""
		for k := range v.tupleLength {
			groupKey = AppendFloatToKey(groupKey, params.GetIndex(
				"latest_groupings_tupindex_"+strconv.Itoa(k), i), v.precision)
		}
		if values, ok = groupings[groupKey]; ok {
			groupings[groupKey] = append(values, latestStateValueSlice[0])
			weightings[groupKey] = append(weightings[groupKey], weight)
		} else {
			groupings[groupKey] = latestStateValueSlice
			weightings[groupKey] = []float64{weight}
		}
		for j := range stateHistory.StateHistoryDepth {
			groupKey = ""
			for k := range v.tupleLength {
				groupingHistory := stateHistories[int(params.GetIndex(
					"grouping_partition_tupindex_"+strconv.Itoa(k), 0))]
				groupKey = AppendFloatToKey(groupKey, groupingHistory.Values.At(
					j, int(params.GetIndex("grouping_value_indices_tupindex_"+
						strconv.Itoa(k), i))), v.precision)
			}
			stateValueSlice := []float64{stateHistory.Values.At(j, int(stateValueIndex))}
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
