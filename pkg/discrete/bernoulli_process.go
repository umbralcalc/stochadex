package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// BernoulliProcessIteration observes a success (or failure) with
// a 1 (or 0) depending on the probability provided for each state
// value index.
type BernoulliProcessIteration struct {
	uniformDist *distuv.Uniform
}

func (b *BernoulliProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.uniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (b *BernoulliProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := stateHistories[partitionIndex].Values.RawRowView(0)
	probs := params.Get("state_value_observation_probs")
	for i := range outputValues {
		if b.uniformDist.Rand() < probs[i] {
			outputValues[i] = 1.0
		} else {
			outputValues[i] = 0.0
		}
	}
	return outputValues
}
