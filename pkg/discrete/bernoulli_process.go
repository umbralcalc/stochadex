package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// BernoulliProcessIteration emits 1/0 based on per-dimension success probs.
//
// Usage hints:
//   - Provide "state_value_observation_probs" with values in [0, 1].
//   - Outputs are written in-place to the current state row and returned.
//   - Seed is taken from the partition's Settings for reproducibility.
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
