package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BinnedIntegrationKernel outputs the configured binned values in time.
type BinnedIntegrationKernel struct {
	binValues   []float64
	binStepsize float64
}

func (b *BinnedIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (b *BinnedIntegrationKernel) SetParams(
	params *simulator.Params,
) {
	b.binValues = params.Get("bin_values")
	b.binStepsize = params.GetIndex("bin_stepsize", 0)
}

func (b *BinnedIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	return b.binValues[int(math.Floor((currentTime-pastTime)/b.binStepsize))]
}
