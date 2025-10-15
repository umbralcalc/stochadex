package kernels

import "github.com/umbralcalc/stochadex/pkg/simulator"

// IntegrationKernel defines the interface for time/state weighting kernels
// used when aggregating over history.
//
// Usage hints:
//   - Configure is called once with the simulator settings.
//   - SetParams reads per-step parameters (e.g., bandwidths, timescales).
//   - Evaluate returns the non-negative weight for a past sample.
type IntegrationKernel interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	SetParams(params *simulator.Params)
	Evaluate(
		currentState []float64,
		pastState []float64,
		currentTime float64,
		pastTime float64,
	) float64
}
