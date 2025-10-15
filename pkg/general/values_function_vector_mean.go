package general

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// UnitValueFunction returns [1]. Combine with "without_normalisation" to
// compute a kernel density (sum of weights) directly.
func UnitValueFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	return []float64{1.0}
}

// PastDiscountedDataValuesFunction reads from "data_values_partition" and
// applies an exponential discount factor raised to the history depth index.
// Useful with a kernel for a past-discounted rolling mean.
func PastDiscountedDataValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		return params.GetCopy("latest_data_values")
	}
	floats.ScaleTo(
		functionValues,
		math.Pow(
			params.GetIndex("past_discounting_factor", 0),
			float64(stateHistoryDepthIndex),
		),
		stateHistory.Values.RawRowView(stateHistoryDepthIndex),
	)
	return functionValues
}

// PastDiscountedOtherValuesFunction mirrors PastDiscountedDataValuesFunction
// for "other_values_partition", optionally subselecting indices.
func PastDiscountedOtherValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("other_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		return params.GetCopy("latest_other_values")
	}
	for i, index := range params.Get("other_values_indices") {
		functionValues[i] = stateHistory.Values.At(stateHistoryDepthIndex, int(index))
	}
	floats.Scale(
		math.Pow(
			params.GetIndex("past_discounting_factor", 0),
			float64(stateHistoryDepthIndex),
		),
		functionValues,
	)
	return functionValues
}

// OtherValuesFunction returns values from "other_values_partition",
// optionally subselecting indices.
func OtherValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("other_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		return params.GetCopy("latest_other_values")
	}
	for i, index := range params.Get("other_values_indices") {
		functionValues[i] = stateHistory.Values.At(stateHistoryDepthIndex, int(index))
	}
	return functionValues
}

// DataValuesVarianceFunction returns per-index squared deviations from the
// provided "mean" for "data_values_partition".
func DataValuesVarianceFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	var varianceValues []float64
	if stateHistoryDepthIndex == -1 {
		varianceValues = params.GetCopy("latest_data_values")
	} else {
		varianceValues = stateHistory.CopyStateRow(stateHistoryDepthIndex)
		if indices, ok := params.GetOk("data_values_indices"); ok {
			values := make([]float64, 0)
			for _, index := range indices {
				values = append(values, varianceValues[int(index)])
			}
			varianceValues = values
		}
	}
	floats.Sub(varianceValues, params.Get("mean"))
	floats.Mul(varianceValues, varianceValues)
	return varianceValues
}

// DataValuesFunction returns values from "data_values_partition",
// optionally subselecting indices.
func DataValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	if stateHistoryDepthIndex == -1 {
		return params.GetCopy("latest_data_values")
	}
	functionValues := stateHistory.CopyStateRow(stateHistoryDepthIndex)
	if indices, ok := params.GetOk("data_values_indices"); ok {
		values := make([]float64, 0)
		for _, index := range indices {
			values = append(values, functionValues[int(index)])
		}
		functionValues = values
	}
	return functionValues
}

// ValuesFunctionVectorMeanIteration computes a kernel-weighted rolling mean
// of a function over historical values and times.
//
// Usage hints:
//   - Provide Function that accepts a history index (-1 for latest) and returns
//     a vector matching the partition state width.
//   - Set Kernel and related params (e.g., discount factors, bandwidth, etc.).
//   - Use "without_normalisation" to return the weighted sum rather than mean;
//     "subtract_from_normalisation" adjusts the normaliser if needed.
type ValuesFunctionVectorMeanIteration struct {
	Function func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		stateHistoryDepthIndex int,
	) []float64
	Kernel kernels.IntegrationKernel
}

func (v *ValuesFunctionVectorMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.Kernel.Configure(partitionIndex, settings)
}

func (v *ValuesFunctionVectorMeanIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	latestStateValues := params.Get("latest_data_values")
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return stateHistories[partitionIndex].Values.RawRowView(0)
	}
	v.Kernel.SetParams(params)
	// convention is to use -1 here as the state history depth index of the
	// very latest function value
	latestFunctionValues := v.Function(params, partitionIndex, stateHistories, -1)
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	cumulativeWeightSum := v.Kernel.Evaluate(
		latestStateValues,
		latestStateValues,
		latestTime,
		latestTime,
	)
	cumulativeWeightedFunctionValueSum := latestFunctionValues
	floats.Scale(cumulativeWeightSum, cumulativeWeightedFunctionValueSum)
	cumulativeWeightedFunctionValueSumVec := mat.NewVecDense(
		stateHistories[partitionIndex].StateWidth,
		cumulativeWeightedFunctionValueSum,
	)
	var weight float64
	sumContributionVec := mat.NewVecDense(
		cumulativeWeightedFunctionValueSumVec.Len(), nil)
	for i := range stateHistory.StateHistoryDepth {
		weight = v.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			timestepsHistory.Values.AtVec(i),
		)
		if weight < 0 {
			panic("negative function weights")
		}
		cumulativeWeightSum += weight
		for j, functionValue := range v.Function(
			params, partitionIndex, stateHistories, i) {
			sumContributionVec.SetVec(j, weight*functionValue)
		}
		cumulativeWeightedFunctionValueSumVec.AddVec(
			cumulativeWeightedFunctionValueSumVec,
			sumContributionVec,
		)
	}
	if d, ok := params.GetOk("without_normalisation"); ok {
		if d[0] == 1 {
			return cumulativeWeightedFunctionValueSumVec.RawVector().Data
		}
	}
	if w, ok := params.GetOk("subtract_from_normalisation"); ok {
		cumulativeWeightSum -= w[0]
	}
	cumulativeWeightedFunctionValueSumVec.ScaleVec(
		1.0/cumulativeWeightSum,
		cumulativeWeightedFunctionValueSumVec,
	)
	return cumulativeWeightedFunctionValueSumVec.RawVector().Data
}
