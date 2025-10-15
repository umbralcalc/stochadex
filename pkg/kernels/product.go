package kernels

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ProductIntegrationKernel multiplies two kernels to form a composite weight.
//
// Usage hints:
//   - Configure and SetParams will be forwarded to both KernelA and KernelB.
//   - Useful to combine, e.g., temporal and state-distance weightings.
type ProductIntegrationKernel struct {
	KernelA IntegrationKernel
	KernelB IntegrationKernel
}

func (p *ProductIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.KernelA.Configure(partitionIndex, settings)
	p.KernelB.Configure(partitionIndex, settings)
}

func (p *ProductIntegrationKernel) SetParams(
	params *simulator.Params,
) {
	p.KernelA.SetParams(params)
	p.KernelB.SetParams(params)
}

func (p *ProductIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	return p.KernelA.Evaluate(
		currentState, pastState, currentTime, pastTime,
	) * p.KernelB.Evaluate(
		currentState, pastState, currentTime, pastTime,
	)
}
