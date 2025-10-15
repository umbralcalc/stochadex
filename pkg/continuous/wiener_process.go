package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// WienerProcessIteration steps a standard Wiener process (Brownian motion)
// per state dimension.
//
// Usage hints:
//   - Provide a per-dimension "variances" param; next increment uses dW ~ N(0, dt).
//   - Ensure the simulation timestep is set appropriately via the timestep function.
//   - Seed is taken from the partition's Settings for reproducibility.
type WienerProcessIteration struct {
	unitNormalDist *distuv.Normal
}

func (w *WienerProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (w *WienerProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			math.Sqrt(params.GetIndex("variances", i)*
				timestepsHistory.NextIncrement)*w.unitNormalDist.Rand()
	}
	return values
}
