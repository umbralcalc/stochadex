package custom

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MovingAverageIteration computes an exponential moving average of
// state values from an upstream partition. It reads the smoothing
// factor alpha from params and blends the latest upstream values
// with the previous moving average.
type MovingAverageIteration struct {
	upstreamPartitionIndex int
}

func (m *MovingAverageIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	m.upstreamPartitionIndex = int(
		settings.Iterations[partitionIndex].Params.Map["upstream_partition"][0],
	)
}

func (m *MovingAverageIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	alpha := params.Map["alpha"][0]
	upstream := stateHistories[m.upstreamPartitionIndex]
	current := stateHistories[partitionIndex]
	stateWidth := current.StateWidth
	output := make([]float64, stateWidth)
	for i := 0; i < stateWidth; i++ {
		prev := current.Values.At(0, i)
		latest := upstream.Values.At(0, i)
		output[i] = alpha*latest + (1.0-alpha)*prev
	}
	return output
}
