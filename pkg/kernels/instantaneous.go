package kernels

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// InstantaneousIntegrationKernel just returns 1.0 for the most
// recent value, else 0.0.
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
