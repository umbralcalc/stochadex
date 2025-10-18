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
	"gonum.org/v1/gonum/stat/distmv"
)

// NormalLikelihoodDistribution models data with a multivariate normal.
//
// Usage hints:
//   - Provide mean/covariance via params or upstream partition outputs.
//   - Optional: "default_covariance" used if provided covariance is not PD.
//   - GenerateNewSamples draws from the current parameterised distribution.
type NormalLikelihoodDistribution struct {
	Src        rand.Source
	mean       *mat.VecDense
	covariance *mat.SymDense
	defaultCov []float64
}

func (n *NormalLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (n *NormalLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	n.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	n.covariance = CovarianceMatrixFromParamsOrPartition(
		params,
		partitionIndex,
		stateHistories,
	)
	if c, ok := params.GetOk("default_covariance"); ok {
		n.defaultCov = c
	}
}

func (n *NormalLikelihoodDistribution) getDist() *distmv.Normal {
	dist, ok := distmv.NewNormal(
		n.mean.RawVector().Data,
		n.covariance,
		n.Src,
	)
	if !ok {
		if n.defaultCov != nil {
			dist, _ = distmv.NewNormal(
				n.mean.RawVector().Data,
				mat.NewSymDense(n.mean.Len(), n.defaultCov),
				n.Src,
			)
		} else {
			panic("covariance matrix is not positive-definite")
		}
	}
	return dist
}

func (n *NormalLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := n.getDist()
	return dist.LogProb(data)
}

func (n *NormalLikelihoodDistribution) GenerateNewSamples() []float64 {
	dist := n.getDist()
	return dist.Rand(nil)
}

func (n *NormalLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	stateWidth := n.mean.Len()
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(n.covariance)
	if !ok {
		panic("cholesky decomp for covariance matrix failed")
	}
	logLikeGrad := mat.NewVecDense(stateWidth, nil)
	diffVector := mat.NewVecDense(
		stateWidth,
		floats.SubTo(make([]float64, stateWidth), data, n.mean.RawVector().Data),
	)
	err := choleskyDecomp.SolveVecTo(logLikeGrad, diffVector)
	if err != nil {
		panic(err)
	}
	return logLikeGrad.RawVector().Data
}
