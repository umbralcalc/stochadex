package kernels

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// InstantaneousIntegrationKernel returns 1.0 for a sample whose time equals
// the current time and 0.0 otherwise.
//
// Usage hints:
//   - Useful to select only the latest value when aggregating.
//
// Update:
//
//	w(s) = 1 if t_s == t, else 0
//
//	where t is the current time the caller passes and t_s the sample's time.
type InstantaneousIntegrationKernel struct{}

// Configure implements IntegrationKernel.
func (i *InstantaneousIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

// SetParams implements IntegrationKernel.
func (i *InstantaneousIntegrationKernel) SetParams(
	params *simulator.Params,
) {
}

// Evaluate implements IntegrationKernel.
func (i *InstantaneousIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	if currentTime == pastTime {
		return 1.0
	} else {
		return 0.0
	}
}
