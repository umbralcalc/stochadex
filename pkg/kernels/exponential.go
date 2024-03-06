package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ExponentialIntegrationKernel is a simple exponentially-weighted
// historical average value.
type ExponentialIntegrationKernel struct {
	timescale float64
}

func (e *ExponentialIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (e *ExponentialIntegrationKernel) SetParams(
	params *simulator.OtherParams,
) {
	e.timescale = params.FloatParams["exponential_weighting_timescale"][0]
}

func (e *ExponentialIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	return math.Exp((pastTime - currentTime) / e.timescale)
}
