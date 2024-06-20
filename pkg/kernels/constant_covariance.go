package kernels

import (
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
	c.stateWidth = settings.StateWidths[partitionIndex]
	c.covMatrix = mat.NewSymDense(c.stateWidth, nil)
	c.SetParams(settings.OtherParams[partitionIndex])
}

func (c *ConstantGaussianCovarianceKernel) SetParams(
	params *simulator.OtherParams,
) {
	row := 0
	col := 0
	upperTri := mat.NewTriDense(c.stateWidth, mat.Upper, nil)
	for i, param := range params.FloatParams["upper_triangle_cholesky_of_cov_matrix"] {
		// nonzero values along the diagonal are needed as a constraint
		if col == row && param == 0.0 {
			param = 1e-4
			params.FloatParams["upper_triangle_cholesky_of_cov_matrix"][i] = param
		}
		upperTri.SetTri(row, col, param)
		col += 1
		if col == c.stateWidth {
			row += 1
			col = row
		}
	}
	var choleskyDecomp mat.Cholesky
	choleskyDecomp.SetFromU(upperTri)
	choleskyDecomp.ToSym(c.covMatrix)
}

func (c *ConstantGaussianCovarianceKernel) GetCovariance(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) *mat.SymDense {
	return c.covMatrix
}
