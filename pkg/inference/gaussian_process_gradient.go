package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// GaussianProcessGradientIteration
type GaussianProcessGradientIteration struct {
	Batch *simulator.StateHistory
}

func (g *GaussianProcessGradientIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {

}

func (g *GaussianProcessGradientIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{}
}

func (g *GaussianProcessGradientIteration) UpdateMemory(
	stateHistory *simulator.StateHistory,
) {
	g.Batch = stateHistory
}
