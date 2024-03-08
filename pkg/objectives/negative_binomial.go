package objectives

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// NegativeBinomialDataLinkingLogLikelihood assumes the real data are well
// described by a negative binomial distribution, given the input mean
// and covariance matrix.
type NegativeBinomialDataLinkingLogLikelihood struct {
	Src rand.Source
}

func (n *NegativeBinomialDataLinkingLogLikelihood) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewSource(settings.Seeds[partitionIndex])
}

func (n *NegativeBinomialDataLinkingLogLikelihood) Evaluate(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	logLike := 0.0
	for i := 0; i < mean.Len(); i++ {
		r := mean.AtVec(i) * mean.AtVec(i) /
			(covariance.At(i, i) - mean.AtVec(i))
		p := mean.AtVec(i) / covariance.At(i, i)
		lg1, _ := math.Lgamma(r + data[i])
		lg2, _ := math.Lgamma(data[i] + 1.0)
		lg3, _ := math.Lgamma(data[i])
		logLike += lg1 + lg2 + lg3 + (r * math.Log(p)) +
			(data[i] * math.Log(1.0-p))
	}
	return logLike
}

func (n *NegativeBinomialDataLinkingLogLikelihood) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	samples := make([]float64, 0)
	distPoisson := &distuv.Poisson{Lambda: 1.0, Src: n.Src}
	distGamma := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: n.Src}
	for i := 0; i < mean.Len(); i++ {
		distGamma.Beta = 1.0 /
			((covariance.At(i, i) / mean.AtVec(i)) - 1.0)
		distGamma.Alpha = mean.AtVec(i) * distGamma.Beta
		distPoisson.Lambda = distGamma.Rand()
		samples = append(samples, distPoisson.Rand())
	}
	return samples
}
