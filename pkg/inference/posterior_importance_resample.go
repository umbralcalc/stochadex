package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
	"gonum.org/v1/gonum/stat/distuv"
)

// PosteriorImportanceResampleIteration updates a sample frpm the posterior
// distribution over params using log-likelihood and param values given in
// the state history of other partitions.
type PosteriorImportanceResampleIteration struct {
	Src     rand.Source
	catDist distuv.Categorical
}

func (p *PosteriorImportanceResampleIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	nilWeights := make(
		[]float64,
		len(settings.OtherParams[partitionIndex].IntParams["loglike_partitions"]),
	)
	nilWeights[0] = 1.0
	p.catDist = distuv.NewCategorical(
		nilWeights,
		rand.NewSource(settings.Seeds[partitionIndex]),
	)
	p.Src = rand.NewSource(settings.Seeds[partitionIndex])
}

func (p *PosteriorImportanceResampleIteration) Iterate(
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
	logNorm := floats.LogSumExp(logLikes)
	for i, logLike := range logLikes {
		p.catDist.Reweight(i, math.Exp(logLike-logNorm))
	}
	sampleCentre := stateHistories[params.IntParams["param_partitions"][int(
		p.catDist.Rand())]].Values.RawRowView(0)
	normDist, ok := distmv.NewNormal(
		sampleCentre,
		mat.NewSymDense(len(sampleCentre), params.FloatParams["sample_covariance"]),
		p.Src,
	)
	if !ok {
		panic("covariance matrix is not positive-definite")
	}
	return normDist.Rand(nil)
}
