package interactions

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Actor is the interface that must be implemented in order to
// perform actions directly on the state of the stochastic process.
type Actor interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	Act(state []float64, action []float64) []float64
}

// DoNothingActor implements an actor which does not ever act.
type DoNothingActor struct{}

func (d *DoNothingActor) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (d *DoNothingActor) Act(
	state []float64,
	action []float64,
) []float64 {
	return state
}
