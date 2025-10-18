package continuous

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// WienerProcessIteration implements a standard Wiener process (Brownian motion)
// for stochastic simulation.
//
// The Wiener process W(t) is a fundamental continuous-time stochastic process
// that serves as the building block for many other stochastic models. It
// represents pure random motion with no drift or mean reversion.
//
// Mathematical Background:
// The Wiener process W(t) is characterized by:
//   - W(0) = 0 (starts at zero)
//   - W(t) - W(s) ~ N(0, t-s) for t > s (independent increments)
//   - W(t) has continuous sample paths (almost surely)
//   - Cov(W(s), W(t)) = min(s,t) (covariance structure)
//
// Implementation:
// At each timestep, the process evolves as:
//
//	X(t+dt) = X(t) + sqrt(variance * dt) * Z
//
// where Z ~ N(0,1) is a standard normal random variable, variance is the
// per-dimension variance rate, and dt is the timestep size.
//
// Applications:
//   - Financial modeling: Asset price dynamics, interest rate modeling
//   - Physics: Particle diffusion, thermal motion, quantum mechanics
//   - Engineering: Noise modeling, signal processing, control systems
//   - Machine learning: Stochastic optimization, sampling algorithms
//
// Configuration:
//   - Provide "variances" parameter: per-dimension variance rates
//   - Set appropriate timestep size via TimestepFunction
//   - Seed controls reproducibility via partition Settings
//
// Example:
//
//	iteration := &WienerProcessIteration{}
//	// Configure with variance = 0.1, dt = 0.01
//	// Results in sqrt(0.1 * 0.01) = 0.0316 standard deviation per step
//
// Performance:
//   - O(d) time complexity where d is the number of dimensions
//   - Memory usage: O(1) per dimension
//   - Efficient for high-dimensional simulations
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
