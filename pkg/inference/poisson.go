package inference

import (
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonLikelihoodDistribution models count data with a Poisson distribution.
//
// Usage hints:
//   - Provide mean via params or upstream partition outputs.
//   - GenerateNewSamples draws iid Poisson variates per dimension.
type PoissonLikelihoodDistribution struct {
	sampler *rng.Sampler
	mean    *mat.VecDense
}

func (p *PoissonLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (p *PoissonLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	p.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
}

func (p *PoissonLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := &distuv.Poisson{Lambda: 1.0}
	logLike := 0.0
	for i := range p.mean.Len() {
		dist.Lambda = p.mean.AtVec(i)
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (p *PoissonLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	for i := range p.mean.Len() {
		samples = append(samples, p.sampler.Poisson(p.mean.AtVec(i)))
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
