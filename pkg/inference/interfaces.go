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
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// LikelihoodDistribution defines a likelihood model over observed data for
// statistical inference and parameter estimation.
//
// This interface represents a probability distribution that can evaluate
// the likelihood of observed data given parameters and generate new samples
// from the distribution. It serves as the foundation for Bayesian inference,
// parameter estimation, and model validation in stochadex simulations.
//
// Mathematical Concept:
// A likelihood distribution represents the probability model p(data | parameters),
// where the likelihood function measures how well the model explains observed data
// given specific parameter values. This is fundamental to:
//   - Maximum likelihood estimation: θ̂ = argmax_θ p(data | θ)
//   - Bayesian inference: p(θ | data) ∝ p(data | θ) × p(θ)
//   - Model comparison and selection
//   - Parameter uncertainty quantification
//
// Interface Methods:
//   - SetSeed: Initialize random number generator state for reproducible sampling
//   - SetParams: Configure distribution parameters from simulation context
//   - EvaluateLogLike: Compute log-likelihood log p(data | params) for given data
//   - GenerateNewSamples: Draw new samples from the current parameter configuration
//
// Implementation Requirements:
//   - SetSeed must be called before any other methods
//   - SetParams must be called before EvaluateLogLike or GenerateNewSamples
//   - EvaluateLogLike should return log-likelihood (not raw likelihood) for numerical stability
//   - GenerateNewSamples should return samples consistent with current parameters
//
// Example Usage:
//
//	dist := &BetaLikelihoodDistribution{}
//	dist.SetSeed(0, settings)
//	dist.SetParams(params, partitionIndex, stateHistories, timestepsHistory)
//
//	// Evaluate likelihood of observed data
//	logLike := dist.EvaluateLogLike(observedData)
//
//	// Generate new samples for validation
//	newSamples := dist.GenerateNewSamples()
//
// Related Types:
//   - See LikelihoodDistributionWithGradient for gradient-based optimization
//   - See BetaLikelihoodDistribution for beta distribution implementation
//   - See NormalLikelihoodDistribution for normal distribution implementation
type LikelihoodDistribution interface {
	SetSeed(partitionIndex int, settings *simulator.Settings)
	SetParams(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	)
	EvaluateLogLike(data []float64) float64
	GenerateNewSamples() []float64
}

// LikelihoodDistributionWithGradient extends LikelihoodDistribution with a
// mean gradient for optimisation.
type LikelihoodDistributionWithGradient interface {
	LikelihoodDistribution
	EvaluateLogLikeMeanGrad(data []float64) []float64
}
