package inference

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
	"scientificgo.org/special"
)

// BetaLikelihoodDistribution assumes the real data are well
// described by a beta distribution, given the input alpha
// and beta parameters.
type BetaLikelihoodDistribution struct {
	Src   rand.Source
	alpha *mat.VecDense
	beta  *mat.VecDense
}

func (b *BetaLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (b *BetaLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	if alpha, ok := params.GetOk("alpha"); ok {
		alphaCopy := make([]float64, len(alpha))
		betaCopy := make([]float64, len(alpha))
		copy(alphaCopy, alpha)
		copy(betaCopy, params.Get("beta"))
		b.alpha = mat.NewVecDense(len(alpha), alphaCopy)
		b.beta = mat.NewVecDense(len(alpha), betaCopy)
	} else {
		b.alpha = mat.NewVecDense(len(alpha), nil)
		b.beta = mat.NewVecDense(len(alpha), nil)
		mean := MeanFromParamsOrPartition(
			params, partitionIndex, stateHistories)
		variance := VarianceFromParams(params)
		var f, m float64
		for i := range mean.Len() {
			m = mean.AtVec(i)
			f = (m * (1.0 - m) / variance.AtVec(i)) - 1.0
			b.alpha.SetVec(i, m*f)
			b.beta.SetVec(i, (1.0-m)*f)
		}
	}
}

func (b *BetaLikelihoodDistribution) EvaluateLogLike(
	data []float64,
) float64 {
	dist := &distuv.Beta{Alpha: 1.0, Beta: 1.0, Src: b.Src}
	logLike := 0.0
	for i := range b.alpha.Len() {
		dist.Alpha = b.alpha.AtVec(i)
		dist.Beta = b.beta.AtVec(i)
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (b *BetaLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Beta{Alpha: 1.0, Beta: 1.0, Src: b.Src}
	for i := range b.alpha.Len() {
		dist.Alpha = b.alpha.AtVec(i)
		dist.Beta = b.beta.AtVec(i)
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (b *BetaLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	var prec, mean, x, alpha float64
	for i := range b.alpha.Len() {
		x = data[i]
		alpha = b.alpha.AtVec(i)
		prec = alpha + b.beta.AtVec(i)
		mean = alpha / prec
		logLikeGrad = append(
			logLikeGrad,
			prec*(math.Log(x/(1.0-x))+
				special.Digamma((1.0-mean)*prec)-
				special.Digamma(mean*prec)),
		)
	}
	return logLikeGrad
}
