package phenomena

import (
	"math"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

func ToUpperTriangular(m *mat.SymDense) []float64 {
	values := make([]float64, 0)
	nrows, ncols := m.Dims()
	diagCol := int(math.Floor(float64(ncols) / 2.0))
	for i := 0; i < nrows; i++ {
		values = append(
			values,
			m.RawSymmetric().Data[(i*ncols)+diagCol:((i+1)*ncols)]...,
		)
	}
	return values
}

// WeightedWindowedCovarianceIteration computes the rolling windowed weighted covariance
// estimate of values specified by another partition using a mean vector also specified by
// another partition.
type WeightedWindowedCovarianceIteration struct {
	Kernel          kernels.IntegrationKernel
	valuesPartition int
	meansPartition  int
}

func (w *WeightedWindowedCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Kernel.Configure(partitionIndex, settings)
	w.valuesPartition = int(settings.OtherParams[partitionIndex].IntParams["values_partition"][0])
	w.meansPartition = int(settings.OtherParams[partitionIndex].IntParams["means_partition"][0])
}

func (w *WeightedWindowedCovarianceIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[w.valuesPartition]
	if timestepsHistory.CurrentStepNumber < stateHistory.StateHistoryDepth {
		return ToUpperTriangular(mat.NewSymDense(stateHistory.StateWidth, nil))
	}
	w.Kernel.SetParams(params)
	var valuesTrans mat.Dense
	valuesTrans.CloneFrom(stateHistory.Values.T())
	means := params.FloatParams["partition_"+strconv.Itoa(w.meansPartition)]
	mostRecentDiffVec := mat.NewVecDense(stateHistory.StateWidth, nil)
	latestStateValues := params.FloatParams["partition_"+strconv.Itoa(w.valuesPartition)]
	latestTime := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
	for j := 0; j < stateHistory.StateWidth; j++ {
		v := valuesTrans.RawRowView(j)
		floats.AddConst(-means[j], v)
		mostRecentDiffVec.SetVec(j, latestStateValues[j]-means[j])
	}
	covMat := mat.NewSymDense(stateHistory.StateWidth, nil)
	sqrtWeights := make([]float64, 0)
	cumulativeWeightSum := 0.0
	var weight float64
	for i := 0; i < stateHistory.StateHistoryDepth; i++ {
		weight = w.Kernel.Evaluate(
			latestStateValues,
			stateHistory.Values.RawRowView(i),
			latestTime,
			timestepsHistory.Values.AtVec(i),
		)
		sqrtWeights = append(sqrtWeights, math.Sqrt(weight))
		cumulativeWeightSum += weight
	}
	mostRecentDiffVec.ScaleVec(
		math.Sqrt(
			w.Kernel.Evaluate(
				latestStateValues,
				stateHistory.Values.RawRowView(0),
				latestTime,
				timestepsHistory.Values.AtVec(0),
			),
		),
		mostRecentDiffVec,
	)
	for j := 0; j < stateHistory.StateWidth; j++ {
		v := valuesTrans.RawRowView(j)
		floats.Mul(v, sqrtWeights)
	}
	covMat.SymOuterK(
		1.0/cumulativeWeightSum,
		valuesTrans.Slice(0, stateHistory.StateWidth, 0, stateHistory.StateHistoryDepth),
	)
	// adding in the most recent weighted values here
	covMat.SymRankOne(covMat, 1.0/cumulativeWeightSum, mostRecentDiffVec)

	// returns the upper triangular part of the covariance matrix
	return ToUpperTriangular(covMat)
}
