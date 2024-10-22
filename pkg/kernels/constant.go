package kernels

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ConstantIntegrationKernel just returns 1.0 for every value.
type ConstantIntegrationKernel struct{}

func (c *ConstantIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (c *ConstantIntegrationKernel) SetParams(
	params *simulator.Params,
) {
}

func (c *ConstantIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	return 1.0
}
