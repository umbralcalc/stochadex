package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// BinomialObservationProcessIteration observes each count value provided
// with a binomial probability distribution - emulating a sequence of
// Bernoulli trials.
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
