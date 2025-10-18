// Package inference provides statistical inference and likelihood modeling
// capabilities for stochadex simulations. It includes probability distributions,
// likelihood functions, gradient computation, and Bayesian inference utilities
// for parameter estimation and model validation.
//
// Key Features:
//   - Probability distribution implementations (Beta, Gamma, Normal, Poisson, etc.)
//   - Likelihood function evaluation and gradient computation
//   - Bayesian inference with posterior estimation
//   - Parameter estimation and optimization support
//   - Statistical testing and validation utilities
//   - Gradient-based optimization algorithms
//
// Mathematical Background:
// The inference package implements statistical models for:
//   - Likelihood functions: p(data | parameters)
//   - Posterior distributions: p(parameters | data) ∝ p(data | parameters) × p(parameters)
//   - Gradient computation: ∇_θ log p(data | parameters)
//   - Parameter estimation: θ̂ = argmax_θ p(data | parameters)
//
// Design Philosophy:
// This package emphasizes modularity and composability, providing building
// blocks for statistical inference that can be combined to create complex
// inference workflows. All distributions implement standard interfaces for
// likelihood evaluation and gradient computation.
//
// Usage Patterns:
//   - Parameter estimation from simulation data
//   - Model validation and goodness-of-fit testing
//   - Bayesian inference with prior distributions
//   - Gradient-based optimization for parameter fitting
//   - Statistical hypothesis testing and model comparison
package inference

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// GammaLikelihoodDistribution assumes the real data are well described
// by a gamma distribution, given the input mean and variance.
type GammaLikelihoodDistribution struct {
	Src      rand.Source
	mean     *mat.VecDense
	variance *mat.VecDense
}

func (g *GammaLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (g *GammaLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	g.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	g.variance = VarianceFromParamsOrPartition(
		params,
		partitionIndex,
		stateHistories,
	)
}

func (g *GammaLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	logLike := 0.0
	var m, v float64
	for i := range g.mean.Len() {
		m = g.mean.AtVec(i)
		v = g.variance.AtVec(i)
		dist.Beta = m / v
		dist.Alpha = m * m / v
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (g *GammaLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Gamma{Alpha: 1.0, Beta: 1.0, Src: g.Src}
	var m, v float64
	for i := range g.mean.Len() {
		m = g.mean.AtVec(i)
		v = g.variance.AtVec(i)
		dist.Beta = m / v
		dist.Alpha = m * m / v
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (g *GammaLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	for i := range g.mean.Len() {
		logLikeGrad = append(
			logLikeGrad,
			(data[i]-g.mean.AtVec(i))/g.variance.AtVec(i),
		)
	}
	return logLikeGrad
}
