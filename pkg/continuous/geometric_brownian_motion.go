package continuous

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// GeometricBrownianMotionIteration steps a multiplicative (geometric)
// Brownian motion per dimension.
//
// Usage hints:
//   - Provide per-dimension "variances"; multiplicative noise uses sqrt(variance*dt).
//   - Consider log-transforms if you need additive dynamics in log space.
//   - Seed is taken from the partition's Settings for reproducibility.
//
// Update:
//
//	x[i] <- x[i] * (1 + sqrt(variances[i] * dt) * Z[i]),  Z[i] ~ N(0, 1)
type GeometricBrownianMotionIteration struct {
	sampler *rng.Sampler
}

// Configure implements simulator.Iteration.
func (g *GeometricBrownianMotionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

// Iterate implements simulator.Iteration.
func (g *GeometricBrownianMotionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := stateHistory.GetNextStateRowToUpdate()
	// Hoist the variances slice out of the loop (params.GetIndex is a per-call map lookup).
	variances := params.Get("variances")
	for i := range values {
		values[i] *= 1.0 +
			math.Sqrt(variances[i]*
				timestepsHistory.NextIncrement)*g.sampler.NormFloat64()
	}
	return values
}
