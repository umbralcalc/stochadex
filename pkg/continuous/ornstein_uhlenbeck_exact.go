package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
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
	unitNormalDist *distuv.Normal
}

func (o *OrnsteinUhlenbeckExactGaussianIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	o.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (o *OrnsteinUhlenbeckExactGaussianIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	dt := timestepsHistory.NextIncrement
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		th := params.GetIndex("thetas", i)
		mu := params.GetIndex("mus", i)
		sig := params.GetIndex("sigmas", i)
		x := stateHistory.Values.At(0, i)
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
		values[i] = mean + math.Sqrt(condVar)*o.unitNormalDist.Rand()
	}
	return values
}
