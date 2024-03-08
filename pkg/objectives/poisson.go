package objectives

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonDataLinkingLogLikelihood assumes the real data are well described
// by a Poisson distribution, given the input mean and covariance matrix.
type PoissonDataLinkingLogLikelihood struct {
	Src rand.Source
}

func (p *PoissonDataLinkingLogLikelihood) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Src = rand.NewSource(settings.Seeds[partitionIndex])
}

func (p *PoissonDataLinkingLogLikelihood) Evaluate(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	dist := &distuv.Poisson{Lambda: 1.0, Src: p.Src}
	logLike := 0.0
	for i := 0; i < mean.Len(); i++ {
		dist.Lambda = mean.AtVec(i)
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (p *PoissonDataLinkingLogLikelihood) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Poisson{Lambda: 1.0, Src: p.Src}
	for i := 0; i < mean.Len(); i++ {
		dist.Lambda = mean.AtVec(i)
		samples = append(samples, dist.Rand())
	}
	return samples
}
