package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// ConstantGaussianCovarianceKernel models a constant covariance.
type ConstantGaussianCovarianceKernel struct {
	covMatrix  *mat.SymDense
	stateWidth int
}

func (c *ConstantGaussianCovarianceKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	c.SetParams(settings.OtherParams[partitionIndex])
}

func (c *ConstantGaussianCovarianceKernel) SetParams(
	params *simulator.OtherParams,
) {
	c.stateWidth = int(math.Sqrt(float64(len(params.FloatParams["covariance_matrix"]))))
	c.covMatrix = mat.NewSymDense(c.stateWidth, params.FloatParams["covariance_matrix"])
}

func (c *ConstantGaussianCovarianceKernel) GetCovariance(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) *mat.SymDense {
	return c.covMatrix
}
