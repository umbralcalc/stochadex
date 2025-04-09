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
	Src  rand.Source
	mean *mat.VecDense
}

func (p *PoissonLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	p.SetParams(&settings.Iterations[partitionIndex].Params)
}

func (p *PoissonLikelihoodDistribution) SetParams(
	params *simulator.Params,
) {
	p.mean = MeanFromParams(params)
}

func (p *PoissonLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := &distuv.Poisson{Lambda: 1.0, Src: p.Src}
	logLike := 0.0
	for i := range p.mean.Len() {
		dist.Lambda = p.mean.AtVec(i)
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (p *PoissonLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Poisson{Lambda: 1.0, Src: p.Src}
	for i := range p.mean.Len() {
		dist.Lambda = p.mean.AtVec(i)
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (p *PoissonLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, p.mean.Len())
	floats.DivTo(logLikeGrad, data, p.mean.RawVector().Data)
	floats.AddConst(-1.0, logLikeGrad)
	return logLikeGrad
}
