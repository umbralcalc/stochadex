package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// GammaLikelihoodDistribution assumes the real data are well described
// by a gamma distribution, given the input mean and covariance matrix.
type GammaLikelihoodDistribution struct {
	Src      rand.Source
	mean     *mat.VecDense
	variance *mat.VecDense
}

func (g *GammaLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
}

func (g *GammaLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	g.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	g.variance = VarianceFromParams(params)
}

func (g *GammaLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	logLike := 0.0
	var m, v float64
	for i := range g.mean.Len() {
		m = g.mean.AtVec(i)
		v = g.variance.AtVec(i)
		dist.Beta = m / v
		dist.Alpha = m * m / v
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (g *GammaLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	var m, v float64
	for i := range g.mean.Len() {
		m = g.mean.AtVec(i)
		v = g.variance.AtVec(i)
		dist.Beta = m / v
		dist.Alpha = m * m / v
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (g *GammaLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	for i := range g.mean.Len() {
		logLikeGrad = append(
			logLikeGrad,
			(data[i]-g.mean.AtVec(i))/g.variance.AtVec(i),
		)
	}
	return logLikeGrad
}
