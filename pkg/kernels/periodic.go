package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PeriodicIntegrationKernel applies periodic (circular) time weighting.
//
// Usage hints:
//   - Provide "periodic_weighting_timescale"; weight = exp(-2 sin^2(dt/2)/tau^2).
//   - Useful for daily/weekly seasonality.
type PeriodicIntegrationKernel struct {
	timescaleSq float64
}

func (p *PeriodicIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *PeriodicIntegrationKernel) SetParams(
	params *simulator.Params,
) {
	timescale := params.GetIndex("periodic_weighting_timescale", 0)
	p.timescaleSq = timescale * timescale
}

func (p *PeriodicIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	sinTerm := math.Sin((currentTime - pastTime) / 2.0)
	return math.Exp(-2.0 * sinTerm * sinTerm / p.timescaleSq)
}
