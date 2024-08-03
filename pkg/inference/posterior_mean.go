package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// PosteriorMeanIteration updates an estimate of the mean of the posterior
// distribution over params using log-likelihood and param values given in
// the state history of other partitions.
type PosteriorMeanIteration struct {
}

func (p *PosteriorMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *PosteriorMeanIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logLikes := make([]float64, 0)
	for i, loglikePartition := range params["loglike_partitions"] {
		var valueIndex int
		if v, ok := params["loglike_indices"]; ok {
			valueIndex = int(v[i])
		} else {
			valueIndex = 0
		}
		logLikes = append(
			logLikes,
			stateHistories[int(loglikePartition)].Values.At(0, valueIndex),
		)
	}
	logNormLatest := floats.LogSumExp(logLikes)
	logNormPast := params["posterior_log_normalisation"][0]
	logNormTotal := floats.LogSumExp([]float64{logNormLatest, logNormPast})
	mean := mat.VecDenseCopyOf(stateHistories[partitionIndex].Values.RowView(0))
	mean.ScaleVec(math.Exp(logNormPast-logNormTotal), mean)
	for i, paramsPartition := range params["param_partitions"] {
		mean.AddScaledVec(
			mean,
			math.Exp(logLikes[i]-logNormTotal),
			stateHistories[int(paramsPartition)].Values.RowView(0),
		)
	}
	return mean.RawVector().Data
}
