package inference

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// NegativeBinomialLikelihoodDistribution assumes the real data are well
// described by a negative binomial distribution, given the input mean
// and variance.
type NegativeBinomialLikelihoodDistribution struct {
	Src      rand.Source
	mean     *mat.VecDense
	variance *mat.VecDense
}

func (n *NegativeBinomialLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (n *NegativeBinomialLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	n.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	n.variance = VarianceFromParams(params)
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
	distPoisson := &distuv.Poisson{Lambda: 1.0, Src: n.Src}
	distGamma := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: n.Src}
	for i := range n.mean.Len() {
		distGamma.Beta = 1.0 /
			((n.variance.AtVec(i) / n.mean.AtVec(i)) - 1.0)
		distGamma.Alpha = n.mean.AtVec(i) * distGamma.Beta
		distPoisson.Lambda = distGamma.Rand()
		samples = append(samples, distPoisson.Rand())
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
