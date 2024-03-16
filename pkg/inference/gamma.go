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
	Src rand.Source
}

func (g *GammaLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Src = rand.NewSource(settings.Seeds[partitionIndex])
}

func (g *GammaLikelihoodDistribution) EvaluateLogLike(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	logLike := 0.0
	for i := 0; i < mean.Len(); i++ {
		dist.Beta = mean.AtVec(i) * covariance.At(i, i)
		dist.Alpha = mean.AtVec(i) *
			mean.AtVec(i) / covariance.At(i, i)
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (g *GammaLikelihoodDistribution) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	for i := 0; i < mean.Len(); i++ {
		dist.Beta = mean.AtVec(i) * covariance.At(i, i)
		dist.Alpha = mean.AtVec(i) *
			mean.AtVec(i) / covariance.At(i, i)
		samples = append(samples, dist.Rand())
	}
	return samples
}
