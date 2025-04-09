package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// NegativeBinomialLikelihoodDistribution assumes the real data are well
// described by a negative binomial distribution, given the input mean
// and covariance matrix.
type NegativeBinomialLikelihoodDistribution struct {
	Src rand.Source
}

func (n *NegativeBinomialLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
}

func (n *NegativeBinomialLikelihoodDistribution) EvaluateLogLike(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	logLike := 0.0
	var r, p, lg1, lg2, lg3 float64
	for i := range mean.Len() {
		r = mean.AtVec(i) * mean.AtVec(i) /
			(covariance.At(i, i) - mean.AtVec(i))
		p = mean.AtVec(i) / covariance.At(i, i)
		lg1, _ = math.Lgamma(r + data[i])
		lg2, _ = math.Lgamma(data[i] + 1.0)
		lg3, _ = math.Lgamma(r)
		logLike += lg1 - lg2 - lg3 + (r * math.Log(p)) +
			(data[i] * math.Log(1.0-p))
	}
	return logLike
}

func (n *NegativeBinomialLikelihoodDistribution) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	samples := make([]float64, 0)
	distPoisson := &distuv.Poisson{Lambda: 1.0, Src: n.Src}
	distGamma := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: n.Src}
	for i := range mean.Len() {
		distGamma.Beta = 1.0 /
			((covariance.At(i, i) / mean.AtVec(i)) - 1.0)
		distGamma.Alpha = mean.AtVec(i) * distGamma.Beta
		distPoisson.Lambda = distGamma.Rand()
		samples = append(samples, distPoisson.Rand())
	}
	return samples
}

func (n *NegativeBinomialLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	var r, m, x float64
	for i := range mean.Len() {
		x = data[i]
		m = mean.AtVec(i)
		r = m * m / (covariance.At(i, i) - m)
		logLikeGrad = append(logLikeGrad, (x/m)-((x+r)/(r+m)))
	}
	return logLikeGrad
}
