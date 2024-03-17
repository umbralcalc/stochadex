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
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logLikes := make([]float64, 0)
	for _, loglikePartition := range params.IntParams["loglike_partitions"] {
		logLikes = append(
			logLikes,
			stateHistories[loglikePartition].Values.At(0, 0),
		)
	}
	normalisation := floats.LogSumExp(logLikes)
	mean := mat.VecDenseCopyOf(stateHistories[partitionIndex].Values.RowView(0))
	mean.ScaleVec(params.FloatParams["past_discounting_factor"][0], mean)
	for i, paramsPartition := range params.IntParams["param_partitions"] {
		mean.AddScaledVec(
			mean,
			math.Exp(logLikes[i]-normalisation),
			stateHistories[paramsPartition].Values.RowView(0),
		)
	}
	return mean.RawVector().Data
}
