package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonProcessIteration steps a simple Poisson counting process.
//
// Usage hints:
//   - Provide per-dimension "rates" (lambda); event approx. prob is rate*dt.
//   - On an event, the count increments by 1; otherwise it is unchanged.
//   - Configure timestep size via the simulator to control event probability.
type PoissonProcessIteration struct {
	unitUniformDist *distuv.Uniform
}

func (p *PoissonProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (p *PoissonProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if params.GetIndex("rates", i) > (params.GetIndex("rates", i)+
			(1.0/timestepsHistory.NextIncrement))*p.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
