package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// LikelihoodDistribution is the interface that must be implemented in
// order to create a likelihood model for some data.
type LikelihoodDistribution interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	SetParams(params *simulator.Params)
	EvaluateLogLike(data []float64) float64
	GenerateNewSamples() []float64
}

// LikelihoodDistributionGradient is the interface that must be implemented in
// order to create a likelihood which computes a gradient.
type LikelihoodDistributionGradient interface {
	LikelihoodDistribution
	EvaluateLogLikeMeanGrad(data []float64) []float64
}
