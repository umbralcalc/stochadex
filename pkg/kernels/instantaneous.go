package kernels

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// InstantaneousIntegrationKernel returns 1.0 for the most recent sample and
// 0.0 otherwise.
//
// Usage hints:
//   - Useful to select only the latest value when aggregating.
type InstantaneousIntegrationKernel struct{}

func (i *InstantaneousIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (i *InstantaneousIntegrationKernel) SetParams(
	params *simulator.Params,
) {
}

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
