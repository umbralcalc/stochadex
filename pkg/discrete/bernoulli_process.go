package discrete

import (
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BernoulliProcessIteration emits 1/0 based on per-dimension success probs.
//
// Usage hints:
//   - Provide "state_value_observation_probs" with values in [0, 1].
//   - Outputs are written into the partition's reusable next-state buffer.
//   - Seed is taken from the partition's Settings for reproducibility.
type BernoulliProcessIteration struct {
	sampler *rng.Sampler
}

func (b *BernoulliProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (b *BernoulliProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := stateHistories[partitionIndex].GetNextStateRowToUpdate()
	probs := params.Get("state_value_observation_probs")
	for i := range outputValues {
		if b.sampler.Float64() < probs[i] {
			outputValues[i] = 1.0
		} else {
			outputValues[i] = 0.0
		}
	}
	return outputValues
}
