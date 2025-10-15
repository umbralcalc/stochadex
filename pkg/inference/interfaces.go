package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// LikelihoodDistribution defines a likelihood model over observed data.
//
// Usage hints:
//   - SetSeed is called once per partition to initialise RNG state.
//   - SetParams configures the distribution from the current simulation context.
//   - EvaluateLogLike computes log p(data | params); GenerateNewSamples draws
//     from the current model.
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
