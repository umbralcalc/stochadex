package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// WeightedWindowedMeanIteration computes the rolling windowed weighted average of values
// specified by another partition.
type WeightedWindowedMeanIteration struct {
	Kernel          kernels.IntegrationKernel
	valuesPartition int
}

func (w *WeightedWindowedMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Kernel.Configure(partitionIndex, settings)
	w.valuesPartition = int(settings.OtherParams[partitionIndex].IntParams["data_values_partition"][0])
}

func (w *WeightedWindowedMeanIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	latestStateValues := params.FloatParams["latest_data_values"]
	stateHistory := stateHistories[w.valuesPartition]
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return latestStateValues
	}
	w.Kernel.SetParams(params)
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	cumulativeWeightSum := w.Kernel.Evaluate(
		latestStateValues,
		latestStateValues,
		latestTime,
		latestTime,
	)
	cumulativeWeightedValueSum := latestStateValues
	floats.Scale(cumulativeWeightSum, cumulativeWeightedValueSum)
	cumulativeWeightedValueSumVec := mat.NewVecDense(
		stateHistory.StateWidth,
		cumulativeWeightedValueSum,
	)
	var weight float64
	sumContributionVec := mat.NewVecDense(cumulativeWeightedValueSumVec.Len(), nil)
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
		weight = w.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			timestepsHistory.Values.AtVec(i),
		)
		if weight < 0 {
			panic("negative mean weights")
		}
		cumulativeWeightSum += weight
		sumContributionVec.ScaleVec(
			weight,
			stateHistory.Values.RowView(i),
		)
		cumulativeWeightedValueSumVec.AddVec(
			cumulativeWeightedValueSumVec,
			sumContributionVec,
		)
	}
	cumulativeWeightedValueSumVec.ScaleVec(
		1.0/cumulativeWeightSum,
		cumulativeWeightedValueSumVec,
	)
	return cumulativeWeightedValueSumVec.RawVector().Data
}
