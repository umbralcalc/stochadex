package continuous

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// OrnsteinUhlenbeckExactGaussianIteration advances each dimension with the
// exact conditional Gaussian transition for an OU process on the current
// timestep increment (independent across dimensions).
//
// For dimension i with θ=thetas[i], μ=mus[i], σ=sigmas[i], Δt=NextIncrement,
// X_{t+Δt} | X_t is Gaussian with mean μ+(X_t-μ)exp(-θΔt) and variance
// (σ²/(2θ))(1-exp(-2θΔt)) when θ>0; when θ=0 it reduces to Brownian motion
// with variance σ²Δt.
//
// Usage hints:
//   - Same param keys as OrnsteinUhlenbeckIteration ("thetas", "mus", "sigmas").
//   - Suitable when Euler–Maruyama error from OrnsteinUhlenbeckIteration would
//     dominate (large θΔt) and the model is linear OU per dimension.
type OrnsteinUhlenbeckExactGaussianIteration struct {
	sampler *rng.Sampler
}

func (o *OrnsteinUhlenbeckExactGaussianIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	o.sampler = rng.New(settings.Iterations[partitionIndex].Seed)
}

func (o *OrnsteinUhlenbeckExactGaussianIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	dt := timestepsHistory.NextIncrement
	values := stateHistory.GetNextStateRowToUpdate()
	// Hoist the per-dimension param slices out of the loop (params.GetIndex is a per-call
	// map lookup); indexing the slices gives identical values.
	thetas := params.Get("thetas")
	mus := params.Get("mus")
	sigmas := params.Get("sigmas")
	for i := range values {
		th := thetas[i]
		mu := mus[i]
		sig := sigmas[i]
		x := values[i]
		var mean, condVar float64
		if th == 0 {
			mean = x
			condVar = sig * sig * dt
		} else {
			e := math.Exp(-th * dt)
			mean = mu + (x-mu)*e
			condVar = (sig * sig / (2 * th)) * (1 - math.Exp(-2*th*dt))
		}
		if condVar < 0 {
			condVar = 0
		}
		values[i] = mean + math.Sqrt(condVar)*o.sampler.NormFloat64()
	}
	return values
}
