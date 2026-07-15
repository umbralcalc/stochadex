package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// NegativeBinomialLikelihoodDistribution assumes the real data are well
// described by a negative binomial distribution, given the input mean
// and variance.
type NegativeBinomialLikelihoodDistribution struct {
	sampler  *rng.Sampler
	mean     *mat.VecDense
	variance *mat.VecDense
}

func (n *NegativeBinomialLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (n *NegativeBinomialLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	n.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	n.variance = VarianceFromParamsOrPartition(
		params,
		partitionIndex,
		stateHistories,
	)
}

func (n *NegativeBinomialLikelihoodDistribution) EvaluateLogLike(
	data []float64,
) float64 {
	logLike := 0.0
	var r, p, lg1, lg2, lg3 float64
	for i := range n.mean.Len() {
		r = n.mean.AtVec(i) * n.mean.AtVec(i) /
			(n.variance.AtVec(i) - n.mean.AtVec(i))
		p = n.mean.AtVec(i) / n.variance.AtVec(i)
		lg1, _ = math.Lgamma(r + data[i])
		lg2, _ = math.Lgamma(data[i] + 1.0)
		lg3, _ = math.Lgamma(r)
		logLike += lg1 - lg2 - lg3 + (r * math.Log(p)) +
			(data[i] * math.Log(1.0-p))
	}
	return logLike
}

func (n *NegativeBinomialLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	for i := range n.mean.Len() {
		// Gamma–Poisson mixture: draw lambda ~ Gamma(shape, rate), then Poisson(lambda).
		// Both draws come from the one sampler, matching the two distuv distributions that
		// previously shared a single Src (order: gamma then Poisson).
		beta := 1.0 / ((n.variance.AtVec(i) / n.mean.AtVec(i)) - 1.0)
		alpha := n.mean.AtVec(i) * beta
		lambda := n.sampler.Gamma(alpha, beta)
		samples = append(samples, n.sampler.Poisson(lambda))
	}
	return samples
}

func (n *NegativeBinomialLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	var r, m, x float64
	for i := range n.mean.Len() {
		x = data[i]
		m = n.mean.AtVec(i)
		r = m * m / (n.variance.AtVec(i) - m)
		logLikeGrad = append(logLikeGrad, (x/m)-((x+r)/(r+m)))
	}
	return logLikeGrad
}
