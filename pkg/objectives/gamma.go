package objectives

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// GammaDataLinkingLogLikelihood assumes the real data are well described
// by a gamma distribution, given the input mean and covariance matrix.
type GammaDataLinkingLogLikelihood struct {
	Src rand.Source
}

func (g *GammaDataLinkingLogLikelihood) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Src = rand.NewSource(settings.Seeds[partitionIndex])
}

func (g *GammaDataLinkingLogLikelihood) Evaluate(
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

func (g *GammaDataLinkingLogLikelihood) GenerateNewSamples(
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
