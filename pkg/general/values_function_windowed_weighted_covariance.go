package general

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// ValuesFunctionWindowedWeightedCovarianceIteration computes the rolling windowed
// weighted covariance value of a function using inputs into the latter specified
// by another partition and weights specified by an integration kernel. It also
// requires a "mean" param vector.
type ValuesFunctionWindowedWeightedCovarianceIteration struct {
	Function func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		stateHistoryDepthIndex int,
	) []float64
	Kernel kernels.IntegrationKernel
}

func (v *ValuesFunctionWindowedWeightedCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	v.Kernel.Configure(partitionIndex, settings)
}

func (v *ValuesFunctionWindowedWeightedCovarianceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[int(params.GetIndex("data_values_partition", 0))]
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return mat.NewSymDense(stateHistory.StateWidth, nil).RawSymmetric().Data
	}
	v.Kernel.SetParams(params)
	latestStateValues := params.Get("latest_data_values")
	// convention is to use -1 here as the state history depth index of the
	// very latest function value
	latestFunctionValues := v.Function(params, partitionIndex, stateHistories, -1)
	functionValuesTrans := mat.NewDense(
		len(latestFunctionValues),
		stateHistory.StateHistoryDepth,
		nil,
	)
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
		functionValuesTrans.SetCol(i, v.Function(
			params, partitionIndex, stateHistories, i))
	}
	mean := params.Get("mean")
	mostRecentDiffVec := mat.NewVecDense(stateHistory.StateWidth, nil)
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	for j := 0; j < stateHistory.StateWidth; j++ {
		f := functionValuesTrans.RawRowView(j)
		floats.AddConst(-mean[j], f)
		mostRecentDiffVec.SetVec(j, latestFunctionValues[j]-mean[j])
	}
	covMat := mat.NewSymDense(stateHistory.StateWidth, nil)
	sqrtWeights := make([]float64, 0)
	cumulativeWeightSum := v.Kernel.Evaluate(
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
		weight = v.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			timestepsHistory.Values.AtVec(i),
		)
		sqrtWeights = append(sqrtWeights, math.Sqrt(weight))
		cumulativeWeightSum += weight
	}
	for j := 0; j < stateHistory.StateWidth; j++ {
		f := functionValuesTrans.RawRowView(j)
		floats.Mul(f, sqrtWeights)
	}
	covMat.SymOuterK(1.0/(cumulativeWeightSum-1), functionValuesTrans)

	// adding in the most recent weighted values here
	covMat.SymRankOne(covMat, 1.0/(cumulativeWeightSum-1), mostRecentDiffVec)

	// returns the upper triangular part of the covariance matrix
	return covMat.RawSymmetric().Data
}
