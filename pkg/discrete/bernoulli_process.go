package discrete

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
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
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (b *BernoulliProcessIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := stateHistories[partitionIndex].Values.RawRowView(0)
	probs := params["state_value_observation_probs"]
	for i := range outputValues {
		if b.uniformDist.Rand() < probs[i] {
			outputValues[i] = 1.0
		} else {
			outputValues[i] = 0.0
		}
	}
	return outputValues
}
