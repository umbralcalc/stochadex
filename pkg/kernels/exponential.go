package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ExponentialIntegrationKernel applies exponential decay over time.
//
// Usage hints:
//   - Provide "exponential_weighting_timescale"; weight = exp((t_past - t_now)/tau).
//   - Suitable for recency-weighted means.
type ExponentialIntegrationKernel struct {
	timescale float64
}

func (e *ExponentialIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (e *ExponentialIntegrationKernel) SetParams(
	params *simulator.Params,
) {
	e.timescale = params.GetIndex("exponential_weighting_timescale", 0)
}

func (e *ExponentialIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	return math.Exp((pastTime - currentTime) / e.timescale)
}
