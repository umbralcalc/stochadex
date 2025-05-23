package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// GeometricBrownianMotionIteration defines an iteration for a simple
// geometric Brownian motion.
type GeometricBrownianMotionIteration struct {
	unitNormalDist *distuv.Normal
}

func (g *GeometricBrownianMotionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (g *GeometricBrownianMotionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) * (1.0 +
			math.Sqrt(params.GetIndex("variances", i)*
				timestepsHistory.NextIncrement)*g.unitNormalDist.Rand())
	}
	return values
}
