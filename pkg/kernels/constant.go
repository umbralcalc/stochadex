package kernels

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ConstantIntegrationKernel returns 1.0 for every sample.
//
// Usage hints:
//   - Use to compute simple (unweighted) sums or means.
type ConstantIntegrationKernel struct{}

// Configure implements IntegrationKernel.
func (c *ConstantIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

// SetParams implements IntegrationKernel.
func (c *ConstantIntegrationKernel) SetParams(
	params *simulator.Params,
) {
}

// Evaluate implements IntegrationKernel.
func (c *ConstantIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	return 1.0
}
