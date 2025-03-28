package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PosteriorKernelUpdateIteration
type PosteriorKernelUpdateIteration struct {
}

func (p *PosteriorKernelUpdateIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {

}

func (p *PosteriorKernelUpdateIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{0.0}
}
