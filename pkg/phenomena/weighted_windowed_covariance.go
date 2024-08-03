package phenomena

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// WeightedWindowedCovarianceIteration computes the rolling windowed weighted covariance
// estimate of values specified by another partition using a mean vector also specified by
// another partition.
type WeightedWindowedCovarianceIteration struct {
	Kernel          kernels.IntegrationKernel
	discount        float64
	valuesPartition int
}

func (w *WeightedWindowedCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Kernel.Configure(partitionIndex, settings)
	w.valuesPartition = int(
		settings.Params[partitionIndex]["data_values_partition"][0])
	if d, ok := settings.Params[partitionIndex]["past_discounting_factor"]; ok {
		w.discount = d[0]
	} else {
		w.discount = 1.0
	}
}

func (w *WeightedWindowedCovarianceIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[w.valuesPartition]
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return mat.NewSymDense(stateHistory.StateWidth, nil).RawSymmetric().Data
	}
	w.Kernel.SetParams(params)
	var valuesTrans mat.Dense
	valuesTrans.CloneFrom(stateHistory.Values.T())
	mean := params["mean"]
	mostRecentDiffVec := mat.NewVecDense(stateHistory.StateWidth, nil)
	latestStateValues := params["latest_data_values"]
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	for j := 0; j < stateHistory.StateWidth; j++ {
		v := valuesTrans.RawRowView(j)
		floats.AddConst(-mean[j], v)
		mostRecentDiffVec.SetVec(j, latestStateValues[j]-mean[j])
	}
	covMat := mat.NewSymDense(stateHistory.StateWidth, nil)
	sqrtWeights := make([]float64, 0)
	cumulativeWeightSum := w.Kernel.Evaluate(
		latestStateValues,
		latestStateValues,
		latestTime,
		latestTime,
	)
	mostRecentDiffVec.ScaleVec(
		math.Sqrt(cumulativeWeightSum),
		mostRecentDiffVec,
	)
	var weight float64
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
		weight = w.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			timestepsHistory.Values.AtVec(i),
		)
		weight *= math.Pow(w.discount, float64(i))
		sqrtWeights = append(sqrtWeights, math.Sqrt(weight))
		cumulativeWeightSum += weight
	}
	for j := 0; j < stateHistory.StateWidth; j++ {
		v := valuesTrans.RawRowView(j)
		floats.Mul(v, sqrtWeights)
	}
	covMat.SymOuterK(1.0/(cumulativeWeightSum-1), &valuesTrans)

	// adding in the most recent weighted values here
	covMat.SymRankOne(covMat, 1.0/(cumulativeWeightSum-1), mostRecentDiffVec)

	// returns the upper triangular part of the covariance matrix
	return covMat.RawSymmetric().Data
}
