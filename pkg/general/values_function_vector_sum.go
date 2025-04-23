package general

import (
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// UnitValueFunction just returns a slice of length 1 with a value of 1. This can be
// used in the ValuesFunctionVectorSumIteration to compute the kernel density directly.
func UnitValueFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	stateHistoryDepthIndex int,
) []float64 {
	return []float64{1.0}
}

// ValuesFunctionTimeDeltaRange defines a past time interval over which to compute the sum.
// Lower limit inclusive, upper limit exclusive.
type ValuesFunctionTimeDeltaRange struct {
	LowerDelta float64
	UpperDelta float64
}

// ValuesFunctionVectorSumIteration computes the rolling windowed weighted
// sum value of a function using inputs into the latter specified by another partition
// and weights specified by an integration kernel.
type ValuesFunctionVectorSumIteration struct {
	Function func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		stateHistoryDepthIndex int,
	) []float64
	Kernel    kernels.IntegrationKernel
	timeRange *ValuesFunctionTimeDeltaRange
}

func (v *ValuesFunctionVectorSumIteration) Configure(
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

func (v *ValuesFunctionVectorSumIteration) Iterate(
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
	var cumulativeWeightedFunctionValueSumVec *mat.VecDense
	var weight, timeDelta, pastTime float64
	if v.timeRange != nil {
		// convention is to use -1 here as the state history depth index of the
		// very latest function value
		latestFunctionValues := v.Function(params, partitionIndex, stateHistories, -1)
		weight = v.Kernel.Evaluate(
			latestStateValues,
			latestStateValues,
			latestTime,
			latestTime,
		)
		cumulativeWeightedFunctionValueSum := latestFunctionValues
		floats.Scale(weight, cumulativeWeightedFunctionValueSum)
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
	sumContributionVec := mat.NewVecDense(
		cumulativeWeightedFunctionValueSumVec.Len(), nil)
	for i := range stateHistory.StateHistoryDepth {
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
		for j, functionValue := range v.Function(
			params, partitionIndex, stateHistories, i) {
			sumContributionVec.SetVec(j, weight*functionValue)
		}
		cumulativeWeightedFunctionValueSumVec.AddVec(
			cumulativeWeightedFunctionValueSumVec,
			sumContributionVec,
		)
	}
	return cumulativeWeightedFunctionValueSumVec.RawVector().Data
}
