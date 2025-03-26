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
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		copy(functionValues, params.Get("latest_data_values"))
		return functionValues
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
	stateHistory := stateHistories[int(params.GetIndex("other_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		copy(functionValues, params.Get("latest_other_values"))
		return functionValues
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

// OtherValuesFunction just returns the value of the "other_values_partition",
// resulting in calculating the rolling windowed weighted mean of the other
// partition values.
func OtherValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("other_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		copy(functionValues, params.Get("latest_other_values"))
		return functionValues
	}
	for i, index := range params.Get("other_values_indices") {
		functionValues[i] = stateHistory.Values.At(stateHistoryDepthIndex, int(index))
	}
	return functionValues
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
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	varianceValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		copy(varianceValues, params.Get("latest_data_values"))
	} else {
		copy(varianceValues, stateHistory.Values.RawRowView(stateHistoryDepthIndex))
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

// DataValuesFunction just returns the value of the "data_values_partition", resulting
// in calculating its rolling windowed weighted mean.
func DataValuesFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	functionValues := make([]float64, stateHistory.StateWidth)
	if stateHistoryDepthIndex == -1 {
		copy(functionValues, params.Get("latest_data_values"))
		return functionValues
	}
	copy(functionValues, stateHistory.Values.RawRowView(stateHistoryDepthIndex))
	if indices, ok := params.GetOk("data_values_indices"); ok {
		values := make([]float64, 0)
		for _, index := range indices {
			values = append(values, functionValues[int(index)])
		}
		functionValues = values
	}
	return functionValues
}

// UnitValueFunction just returns a slice of length 1 with a value of 1. This can be
// used in the ValuesFunctionVectorMeanIteration to compute the kernel density directly.
func UnitValueFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	return []float64{1.0}
}

// ValuesFunctionTimeDeltaRange defines a past time interval over which to compute the mean.
// Lower limit inclusive, upper limit exclusive.
type ValuesFunctionTimeDeltaRange struct {
	LowerDelta float64
	UpperDelta float64
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
	Kernel    kernels.IntegrationKernel
	timeRange *ValuesFunctionTimeDeltaRange
}

func (v *ValuesFunctionVectorMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	if lowerUpper, ok := settings.Iterations[partitionIndex].Params.GetOk(
		"delta_time_range"); ok {
		v.timeRange = &ValuesFunctionTimeDeltaRange{
			LowerDelta: lowerUpper[0],
			UpperDelta: lowerUpper[1],
		}
	}
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
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	cumulativeWeightSum := 0.0
	var cumulativeWeightedFunctionValueSumVec *mat.VecDense
	if v.timeRange != nil {
		// convention is to use -1 here as the state history depth index of the
		// very latest function value
		latestFunctionValues := v.Function(params, partitionIndex, stateHistories, -1)
		cumulativeWeightSum = v.Kernel.Evaluate(
			latestStateValues,
			latestStateValues,
			latestTime,
			latestTime,
		)
		cumulativeWeightedFunctionValueSum := latestFunctionValues
		floats.Scale(cumulativeWeightSum, cumulativeWeightedFunctionValueSum)
		cumulativeWeightedFunctionValueSumVec = mat.NewVecDense(
			stateHistories[partitionIndex].StateWidth,
			cumulativeWeightedFunctionValueSum,
		)
	} else {
		cumulativeWeightedFunctionValueSumVec = mat.NewVecDense(
			stateHistories[partitionIndex].StateWidth,
			nil,
		)
	}
	var weight, timeDelta, pastTime float64
	sumContributionVec := mat.NewVecDense(
		cumulativeWeightedFunctionValueSumVec.Len(), nil)
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
		pastTime = timestepsHistory.Values.AtVec(i)
		if v.timeRange != nil {
			timeDelta = latestTime - pastTime
			if v.timeRange.LowerDelta > timeDelta ||
				timeDelta >= v.timeRange.UpperDelta {
				continue
			}
		}
		weight = v.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			pastTime,
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
