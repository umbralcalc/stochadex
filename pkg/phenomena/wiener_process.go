package phenomena

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// WienerProcessIteration defines an iteration for a simple Wiener
// process.
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
		Src:   rand.NewSource(settings.Seeds[partitionIndex]),
	}
}

func (w *WienerProcessIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			math.Sqrt(params["variances"][i]*
				timestepsHistory.NextIncrement)*w.unitNormalDist.Rand()
	}
	return values
}
