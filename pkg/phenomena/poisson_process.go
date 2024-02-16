package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonProcessIteration defines an iteration for a simple Poisson
// process.
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
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (p *PoissonProcessIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if params.FloatParams["rates"][i] > (params.FloatParams["rates"][i]+
			(1.0/timestepsHistory.NextIncrement))*p.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
