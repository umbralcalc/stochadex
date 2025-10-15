package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// CoxProcessIteration steps a Cox process (doubly stochastic Poisson).
//
// Usage hints:
//   - Provide time-varying "rates" per dimension; event prob approx. rate*dt.
//   - At each step, increments by 1 with the above probability.
//   - Seed is taken from the partition's Settings for reproducibility.
type CoxProcessIteration struct {
	unitUniformDist *distuv.Uniform
}

func (c *CoxProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (c *CoxProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	rates := params.Get("rates")
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if rates[i] > (rates[i]+
			(1.0/timestepsHistory.NextIncrement))*c.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
