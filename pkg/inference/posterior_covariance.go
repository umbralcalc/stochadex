package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// PosteriorCovarianceIteration updates an estimate of the covariance matrix
// of the posterior distribution over params using log-likelihood and param
// values given in the state history of other partitions, and a mean vector.
type PosteriorCovarianceIteration struct {
}

func (p *PosteriorCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *PosteriorCovarianceIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logLikes := make([]float64, 0)
	for i, loglikePartition := range params.IntParams["loglike_partitions"] {
		var valueIndex int
		if v, ok := params.IntParams["loglike_indices"]; ok {
			valueIndex = int(v[i])
		} else {
			valueIndex = 0
		}
		logLikes = append(
			logLikes,
			stateHistories[loglikePartition].Values.At(0, valueIndex),
		)
	}
	logNormLatest := floats.LogSumExp(logLikes)
	logNormPast := params.FloatParams["posterior_log_normalisation"][0]
	logNormTotal := floats.LogSumExp([]float64{logNormLatest, logNormPast})
	dims := len(params.FloatParams["mean"])
	covMat := mat.NewSymDense(dims, stateHistories[partitionIndex].Values.RawRowView(0))
	covMat.ScaleSym(math.Exp(logNormPast-logNormTotal), covMat)
	mean := mat.NewVecDense(dims, params.FloatParams["mean"])
	diffs := mat.NewVecDense(dims, nil)
	for i, paramsPartition := range params.IntParams["param_partitions"] {
		diffs.SubVec(mean, stateHistories[paramsPartition].Values.RowView(0))
		covMat.SymRankOne(
			covMat,
			math.Exp(logLikes[i]-logNormTotal),
			diffs,
		)
	}
	return covMat.RawSymmetric().Data
}
