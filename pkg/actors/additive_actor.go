package actors

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AdditiveActor adds the action values directly to the state.
type AdditiveActor struct {
}

func (a *AdditiveActor) Configure(partitionIndex int, settings *simulator.Settings) {
}

func (a *AdditiveActor) Act(state []float64, action []float64) []float64 {
	for i := range state {
		state[i] += action[i]
	}
	return state
}
