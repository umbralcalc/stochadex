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
	Src  rand.Source
	mean *mat.VecDense
}

func (p *PoissonLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
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
