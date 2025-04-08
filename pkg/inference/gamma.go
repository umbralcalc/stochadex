package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// checkCovariancesImplemented checks whether covariances have been implemented
// for the distribution and panics if not switched off and unimplemented.
func checkCovariancesImplemented(name string, params simulator.Params) {
	if c, ok := params.GetOk("copulas_off"); ok {
		if c[0] != 1 {
			panic(name + " likelihood: copulas not yet implemented for covariances," +
				" must set 'copulas_off' params = 1")
		}
	} else {
		panic(name + " likelihood: copulas not yet implemented for covariances," +
			" must set 'copulas_off' params = 1")
	}
}

// GammaLikelihoodDistribution assumes the real data are well described
// by a gamma distribution, given the input mean and covariance matrix.
type GammaLikelihoodDistribution struct {
	Src rand.Source
}

func (g *GammaLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	checkCovariancesImplemented(
		"gamma", settings.Iterations[partitionIndex].Params)
}

func (g *GammaLikelihoodDistribution) EvaluateLogLike(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	logLike := 0.0
	var m, v float64
	for i := range mean.Len() {
		m = mean.AtVec(i)
		v = covariance.At(i, i)
		dist.Beta = m / v
		dist.Alpha = m * m / v
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
	var m, v float64
	for i := range mean.Len() {
		m = mean.AtVec(i)
		v = covariance.At(i, i)
		dist.Beta = m / v
		dist.Alpha = m * m / v
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (g *GammaLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	for i := range mean.Len() {
		logLikeGrad = append(
			logLikeGrad,
			(data[i]-mean.AtVec(i))/covariance.At(i, i),
		)
	}
	return logLikeGrad
}
