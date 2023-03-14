package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonProcessIteration defines an iteration for a simple Poisson
// process.
type PoissonProcessIteration struct {
	unitUniformDist *distuv.Uniform
}

func (p *PoissonProcessIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if otherParams.FloatParams["rates"][i] > (otherParams.FloatParams["rates"][i]+
			(1.0/timestepsHistory.NextIncrement))*p.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return &simulator.State{
		Values: mat.NewVecDense(
			stateHistory.StateWidth,
			values,
		),
		StateWidth: stateHistory.StateWidth,
	}
}

// NewPoissonProcessIteration creates a new PoissonProcessIteration given a seed.
func NewPoissonProcessIteration(seed uint64) *PoissonProcessIteration {
	return &PoissonProcessIteration{
		unitUniformDist: &distuv.Uniform{
			Min: 0.0,
			Max: 1.0,
			Src: rand.NewSource(seed),
		},
	}
}
