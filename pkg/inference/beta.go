package inference

import (
	"math"

	"math/rand/v2"

	"github.com/scientificgo/special"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// BetaLikelihoodDistribution implements a Beta distribution likelihood model
// for bounded continuous data analysis and parameter estimation.
//
// The Beta distribution is particularly useful for modeling data that is
// bounded between 0 and 1, such as proportions, probabilities, rates, and
// normalized measurements. It provides flexible shape modeling through its
// two shape parameters α (alpha) and β (beta).
//
// Mathematical Background:
// The Beta distribution Beta(α, β) has probability density function:
//
//	f(x | α, β) = x^(α-1) * (1-x)^(β-1) / B(α, β)
//
// where B(α, β) is the Beta function, and x ∈ [0, 1].
//
// Key Properties:
//   - Support: x ∈ [0, 1] (bounded continuous)
//   - Shape parameters: α > 0, β > 0 (both must be positive)
//   - Mean: E[X] = α / (α + β)
//   - Variance: Var[X] = (α * β) / ((α + β)² * (α + β + 1))
//   - Special cases:
//   - α = β = 1: Uniform distribution on [0, 1]
//   - α = 1, β = 1: Uniform distribution
//   - α > 1, β > 1: Bell-shaped distribution
//   - α < 1, β < 1: U-shaped distribution
//
// Applications:
//   - Proportion modeling: Success rates, conversion rates, market shares
//   - Probability estimation: Bayesian inference with Beta priors
//   - Quality control: Defect rates, pass/fail proportions
//   - Financial modeling: Recovery rates, default probabilities
//   - Machine learning: Classification confidence scores
//
// Parameter Configuration:
// The distribution can be configured in two ways:
//  1. Direct parameters: Provide "alpha" and "beta" parameters directly
//  2. Mean-variance: Provide "mean" and "variance" parameters for automatic conversion
//
// Example:
//
//	dist := &BetaLikelihoodDistribution{}
//	dist.SetSeed(0, settings)
//	// Configure with mean=0.3, variance=0.05
//	dist.SetParams(params, partitionIndex, stateHistories, timestepsHistory)
//
//	// Evaluate likelihood of observed proportions
//	logLike := dist.EvaluateLogLike([]float64{0.25, 0.35, 0.28})
//
// Performance:
//   - O(d) time complexity where d is the data dimension
//   - Memory usage: O(d) for parameter storage
//   - Efficient for moderate dimensions (< 1000)
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
	if alphaCopy, ok := params.GetCopyOk("alpha"); ok {
		betaCopy := params.GetCopy("beta")
		b.alpha = mat.NewVecDense(len(alphaCopy), alphaCopy)
		b.beta = mat.NewVecDense(len(alphaCopy), betaCopy)
	} else {
		mean := MeanFromParamsOrPartition(
			params, partitionIndex, stateHistories)
		variance := VarianceFromParamsOrPartition(
			params, partitionIndex, stateHistories)
		b.alpha = mat.NewVecDense(mean.Len(), nil)
		b.beta = mat.NewVecDense(mean.Len(), nil)
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
