package phenomena

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// WeightedWindowedFunctionIteration computes the rolling windowed weighted function
// value specified by another partition.
type WeightedWindowedFunctionIteration struct {
	Kernel                  kernels.IntegrationKernel
	discount                float64
	valuesPartition         int
	functionValuesPartition int
	functionValuesIndices   []int
}

func (w *WeightedWindowedFunctionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Kernel.Configure(partitionIndex, settings)
	w.valuesPartition = int(
		settings.Params[partitionIndex]["data_values_partition"][0])
	w.functionValuesPartition = int(
		settings.Params[partitionIndex]["function_values_partition"][0])
	w.functionValuesIndices = make([]int, 0)
	for _, index := range settings.Params[partitionIndex]["function_values_indices"] {
		w.functionValuesIndices = append(w.functionValuesIndices, int(index))
	}
	if d, ok := settings.Params[partitionIndex]["past_discounting_factor"]; ok {
		w.discount = d[0]
	} else {
		w.discount = 1.0
	}
}

func (w *WeightedWindowedFunctionIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	latestStateValues := params["latest_data_values"]
	latestFunctionValues := params["latest_function_values"]
	stateHistory := stateHistories[w.valuesPartition]
	functionHistory := stateHistories[w.functionValuesPartition]
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return latestFunctionValues
	}
	w.Kernel.SetParams(params)
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	cumulativeWeightSum := w.Kernel.Evaluate(
		latestStateValues,
		latestStateValues,
		latestTime,
		latestTime,
	)
	cumulativeWeightedFunctionValueSum := latestFunctionValues
	floats.Scale(cumulativeWeightSum, cumulativeWeightedFunctionValueSum)
	cumulativeWeightedFunctionValueSumVec := mat.NewVecDense(
		len(w.functionValuesIndices),
		cumulativeWeightedFunctionValueSum,
	)
	var weight float64
	sumContributionVec := mat.NewVecDense(
		cumulativeWeightedFunctionValueSumVec.Len(), nil)
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
		weight = w.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			timestepsHistory.Values.AtVec(i),
		)
		if weight < 0 {
			panic("negative function weights")
		}
		weight *= math.Pow(w.discount, float64(i))
		cumulativeWeightSum += weight
		for j, index := range w.functionValuesIndices {
			sumContributionVec.SetVec(j, weight*functionHistory.Values.At(i, int(index)))
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
