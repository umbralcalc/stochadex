package continuous

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// OrnsteinUhlenbeckIteration steps an Ornstein–Uhlenbeck mean-reverting
// process per dimension using an Euler–Maruyama discretisation.
//
// Usage hints:
//   - Required params per dimension: "thetas" (reversion speed), "mus" (long-run mean),
//     and "sigmas" (volatility).
//   - Timestep size influences both drift and diffusion terms; ensure dt is configured.
//   - Stability: keep θ·Δt modest—large θΔt with EM can bias paths and distort
//     likelihoods versus the continuous-time OU. For inference with stiff θ,
//     prefer OrnsteinUhlenbeckExactGaussianIteration or a smaller Δt.
//   - Seed is taken from the partition's Settings for reproducibility.
type OrnsteinUhlenbeckIteration struct {
	sampler *rng.Sampler
}

func (o *OrnsteinUhlenbeckIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	o.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (o *OrnsteinUhlenbeckIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := stateHistory.GetNextStateRowToUpdate()
	// Hoist the per-dimension param slices (and the shared sqrt(dt)) out of the loop:
	// params.GetIndex is a string-keyed map lookup, so reading them per element per step
	// dominates the cost. Values are identical to the per-element GetIndex form.
	thetas := params.Get("thetas")
	mus := params.Get("mus")
	sigmas := params.Get("sigmas")
	dt := timestepsHistory.NextIncrement
	sqrtDt := math.Sqrt(dt)
	for i := range values {
		values[i] += thetas[i]*(mus[i]-values[i])*dt +
			sigmas[i]*sqrtDt*o.sampler.NormFloat64()
	}
	return values
}
