package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// LikelihoodDistribution is the interface that must be implemented in
// order to create a likelihood model for some data.
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

// LikelihoodDistributionWithGradient is the interface that must be
// implemented in order to create a likelihood which computes a gradient.
type LikelihoodDistributionWithGradient interface {
	LikelihoodDistribution
	EvaluateLogLikeMeanGrad(data []float64) []float64
}

// LikelihoodDistributionWithUpdate is the interface that must be
// implemented in order to create a likelihood which updates itself from data.
type LikelihoodDistributionWithUpdate interface {
	LikelihoodDistribution
	EvaluateUpdate(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []float64
}
