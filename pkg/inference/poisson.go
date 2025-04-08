package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonLikelihoodDistribution assumes the real data are well described
// by a Poisson distribution, given the input mean and covariance matrix.
type PoissonLikelihoodDistribution struct {
	Src rand.Source
}

func (p *PoissonLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	checkCovariancesImplemented(
		"poisson", settings.Iterations[partitionIndex].Params)
}

func (p *PoissonLikelihoodDistribution) EvaluateLogLike(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	dist := &distuv.Poisson{Lambda: 1.0, Src: p.Src}
	logLike := 0.0
	for i := range mean.Len() {
		dist.Lambda = mean.AtVec(i)
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (p *PoissonLikelihoodDistribution) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Poisson{Lambda: 1.0, Src: p.Src}
	for i := range mean.Len() {
		dist.Lambda = mean.AtVec(i)
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (p *PoissonLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, mean.Len())
	floats.DivTo(logLikeGrad, data, mean.RawVector().Data)
	floats.AddConst(-1.0, logLikeGrad)
	return logLikeGrad
}
