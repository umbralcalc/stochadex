package kernels

import "github.com/umbralcalc/stochadex/pkg/simulator"

// IntegrationKernel defines an interface that must be implemented
// for any integration kernel.
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
