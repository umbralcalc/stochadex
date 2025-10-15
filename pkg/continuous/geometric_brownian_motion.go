package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// GeometricBrownianMotionIteration steps a multiplicative (geometric)
// Brownian motion per dimension.
//
// Usage hints:
//   - Provide per-dimension "variances"; multiplicative noise uses sqrt(variance*dt).
//   - Consider log-transforms if you need additive dynamics in log space.
//   - Seed is taken from the partition's Settings for reproducibility.
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
