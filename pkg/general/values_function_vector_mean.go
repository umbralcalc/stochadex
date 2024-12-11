package general

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// PastDiscountedDataValuesFunction returns the value from the "data_values_partition"
// discounted by some "past_discounting_factor" in the params, resulting in
// calculating the past-discounted rolling windowed weighted mean.
func PastDiscountedDataValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	if stateHistoryDepthIndex == -1 {
		return params.Get("latest_data_values")
	}
	functionValue := stateHistories[int(
		params.GetIndex("data_values_partition", 0))].Values.RawRowView(stateHistoryDepthIndex)
	floats.Scale(
		math.Pow(
			params.GetIndex("past_discounting_factor", 0),
			float64(stateHistoryDepthIndex),
		),
		functionValue,
	)
	return functionValue
}

// PastDiscountedOtherValuesFunction just returns the value of the
// "other_values_partition" discounted by some "past_discounting_factor"
// in the params, resulting in calculating the past-discounted rolling
// windowed weighted mean of the other partition values.
func PastDiscountedOtherValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	if stateHistoryDepthIndex == -1 {
		return params.Get("latest_other_values")
	}
	values := make([]float64, 0)
	for _, index := range params.Get("other_values_indices") {
		values = append(
			values,
			stateHistories[int(
				params.GetIndex("other_values_partition", 0))].Values.At(
				stateHistoryDepthIndex, int(index)),
		)
	}
	floats.Scale(
		math.Pow(
			params.GetIndex("past_discounting_factor", 0),
			float64(stateHistoryDepthIndex),
		),
		values,
	)
	return values
}

// OtherValuesFunction just returns the value of the "other_values_partition",
// resulting in calculating the rolling windowed weighted mean of the other
// partition values.
func OtherValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	if stateHistoryDepthIndex == -1 {
		return params.Get("latest_other_values")
	}
	values := make([]float64, 0)
	for _, index := range params.Get("other_values_indices") {
		values = append(
			values,
			stateHistories[int(
				params.GetIndex("other_values_partition", 0))].Values.At(
				stateHistoryDepthIndex, int(index)),
		)
	}
	return values
}

// DataValuesVarianceFunction just returns the contribution to the value of the
// variance of the "data_values_partition", resulting in calculating its rolling
// windowed weighted variance.
func DataValuesVarianceFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	var varianceValue []float64
	if stateHistoryDepthIndex == -1 {
		varianceValue = params.Get("latest_data_values")
	} else {
		varianceValue = stateHistories[int(
			params.GetIndex("data_values_partition", 0))].Values.RawRowView(
			stateHistoryDepthIndex)
		if indices, ok := params.GetOk("data_values_indices"); ok {
			values := make([]float64, 0)
			for _, index := range indices {
				values = append(values, varianceValue[int(index)])
			}
			varianceValue = values
		}
	}
	floats.Sub(varianceValue, params.Get("mean"))
	floats.Mul(varianceValue, varianceValue)
	return varianceValue
}

// DataValuesFunction just returns the value of the "data_values_partition", resulting
// in calculating its rolling windowed weighted mean.
func DataValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	if stateHistoryDepthIndex == -1 {
		return params.Get("latest_data_values")
	}
	dataValues := stateHistories[int(
		params.GetIndex("data_values_partition", 0))].Values.RawRowView(stateHistoryDepthIndex)
	if indices, ok := params.GetOk("data_values_indices"); ok {
		values := make([]float64, 0)
		for _, index := range indices {
			values = append(values, dataValues[int(index)])
		}
		dataValues = values
	}
	return dataValues
}

// ValuesFunctionVectorMeanIteration computes the rolling windowed weighted
// mean value of a function using inputs into the latter specified by another partition
// and weights specified by an integration kernel.
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
	// convention is to use -1 here as the state history depth index of the
	// very latest function value
	latestFunctionValues := v.Function(params, partitionIndex, stateHistories, -1)
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return latestFunctionValues
	}
	v.Kernel.SetParams(params)
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
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
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
	cumulativeWeightedFunctionValueSumVec.ScaleVec(
		1.0/cumulativeWeightSum,
		cumulativeWeightedFunctionValueSumVec,
	)
	return cumulativeWeightedFunctionValueSumVec.RawVector().Data
}
