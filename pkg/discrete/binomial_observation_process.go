package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// BinomialObservationProcessIteration draws binomial counts for selected indices.
//
// Usage hints:
//   - Provide: "observed_values" (counts), "state_value_observation_probs" (p's),
//     and "state_value_observation_indices" (which entries to observe).
//   - For the j-th entry of the indices: draws
//     Binomial(N=observed_values[indices[j]], p=probs[j]); the output has one
//     element per index.
//   - Seed is taken from the partition's Settings for reproducibility.
//
// Update:
//
//	x[j] <- Binomial(observed_values[state_value_observation_indices[j]], state_value_observation_probs[j])
//	n = len(state_value_observation_indices); j runs over 0..n-1.
type BinomialObservationProcessIteration struct {
	binomialDist *distuv.Binomial
}

// Configure implements simulator.Iteration.
func (b *BinomialObservationProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.binomialDist = &distuv.Binomial{
		N: 0,
		P: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

// Iterate implements simulator.Iteration.
func (b *BinomialObservationProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	stateValues := params.Get("observed_values")
	probs := params.Get("state_value_observation_probs")
	for i, index := range params.Get("state_value_observation_indices") {
		b.binomialDist.N = stateValues[int(index)]
		b.binomialDist.P = probs[i]
		outputValues = append(outputValues, b.binomialDist.Rand())
	}
	return outputValues
}
