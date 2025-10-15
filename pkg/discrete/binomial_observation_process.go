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
//   - For each index i: draws Binomial(N=observed_values[i], p=probs[i]).
//   - Seed is taken from the partition's Settings for reproducibility.
type BinomialObservationProcessIteration struct {
	binomialDist *distuv.Binomial
}

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
