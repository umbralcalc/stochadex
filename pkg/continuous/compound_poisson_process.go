package continuous

import (
	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// JumpDistribution defines the interface to draw sudden jumps.
//
// Usage hints:
//   - Implement for custom jump magnitudes; called when a jump event occurs.
//   - Used by compound Poisson processes and drift–jump–diffusions.
type JumpDistribution interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	NewJump(params *simulator.Params, valueIndex int) float64
}

// GammaJumpDistribution draws jump magnitudes from a gamma distribution.
//
// Usage hints:
//   - Param names per dimension: "gamma_alphas" and "gamma_betas".
//   - Seed is taken from the partition's Settings for reproducibility.
type GammaJumpDistribution struct {
	sampler *rng.Sampler
}

func (g *GammaJumpDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (g *GammaJumpDistribution) NewJump(
	params *simulator.Params,
	valueIndex int,
) float64 {
	return g.sampler.Gamma(
		params.GetIndex("gamma_alphas", valueIndex),
		params.GetIndex("gamma_betas", valueIndex),
	)
}

// CompoundPoissonProcessIteration steps a compound Poisson process.
//
// Usage hints:
//   - Provide per-dimension "rates" and a JumpDistribution implementation.
//   - At each step, increments by a jump draw with probability approx. rate*dt.
//   - Configure timestep size via the simulator to control event frequency.
type CompoundPoissonProcessIteration struct {
	JumpDist JumpDistribution
	sampler  *rng.Sampler
}

func (c *CompoundPoissonProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.JumpDist.Configure(partitionIndex, settings)
	c.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (c *CompoundPoissonProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := stateHistory.GetNextStateRowToUpdate()
	// Hoist the rates slice out of the loop (params.GetIndex is a per-call map lookup).
	rates := params.Get("rates")
	for i := range values {
		if rates[i] > (rates[i]+
			(1.0/timestepsHistory.NextIncrement))*c.sampler.Float64() {
			values[i] += c.JumpDist.NewJump(params, i)
		}
	}
	return values
}
