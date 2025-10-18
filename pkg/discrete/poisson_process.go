package discrete

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// PoissonProcessIteration implements a Poisson counting process for event simulation.
//
// The Poisson process is a fundamental counting process that models the occurrence
// of random events in continuous time. It is widely used in queueing theory,
// reliability analysis, and event-driven modeling.
//
// Domain Context:
// Poisson processes model random events occurring independently in time.
// Common applications include:
//   - Arrival times in queueing systems (customers, packets, calls)
//   - Radioactive decay events (particle emissions, nuclear decay)
//   - Network packet arrivals (data transmission, network traffic)
//   - Insurance claim arrivals (accidents, claims processing)
//   - Manufacturing defects (quality control, failure events)
//
// Mathematical Properties:
// The Poisson process N(t) with rate λ has the following properties:
//   - N(0) = 0 (starts at zero)
//   - N(t) - N(s) ~ Poisson(λ(t-s)) for t > s (independent increments)
//   - Inter-arrival times are exponentially distributed with rate λ
//   - Number of events in [0,t] follows Poisson(λt) distribution
//   - Events occur at rate λ per unit time on average
//
// Implementation Details:
//   - Probability of event in timestep dt ≈ λ * dt (for small dt)
//   - Uses uniform random sampling for event detection
//   - Maintains cumulative count of events since t=0
//   - Each dimension can have different event rates
//
// Configuration:
//   - Provide "rates" parameter: per-dimension event rates (λ values)
//   - Set timestep size via TimestepFunction to control event probability
//   - Seed controls reproducibility via partition Settings
//
// Example:
//
//	iteration := &PoissonProcessIteration{}
//	// Configure with rate = 0.5, dt = 0.01
//	// Event probability per step ≈ 0.5 * 0.01 = 0.005 (0.5%)
//
// Performance:
//   - O(d) time complexity where d is the number of dimensions
//   - Memory usage: O(1) per dimension
//   - Efficient for high-dimensional event modeling
type PoissonProcessIteration struct {
	unitUniformDist *distuv.Uniform
}

func (p *PoissonProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (p *PoissonProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if params.GetIndex("rates", i) > (params.GetIndex("rates", i)+
			(1.0/timestepsHistory.NextIncrement))*p.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return values
}
