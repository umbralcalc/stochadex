package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// GaussianIntegrationKernel applies a Gaussian kernel with constant
// covariance matrix.
type GaussianIntegrationKernel struct {
	choleskyDecomp mat.Cholesky
	stateWidth     int
}

func (g *GaussianIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.stateWidth = settings.StateWidths[partitionIndex]
	g.SetParams(settings.OtherParams[partitionIndex])
}

func (g *GaussianIntegrationKernel) SetParams(params *simulator.OtherParams) {
	row := 0
	col := 0
	covMatrix := mat.NewSymDense(g.stateWidth, nil)
	upperTri := mat.NewTriDense(g.stateWidth, mat.Upper, nil)
	for i, param := range params.FloatParams["upper_triangle_cholesky_of_cov_matrix"] {
		// nonzero values along the diagonal are needed as a constraint
		if col == row && param == 0.0 {
			param = 1e-4
			params.FloatParams["upper_triangle_cholesky_of_cov_matrix"][i] = param
		}
		upperTri.SetTri(row, col, param)
		col += 1
		if col == g.stateWidth {
			row += 1
			col = row
		}
	}
	var choleskyDecomp mat.Cholesky
	choleskyDecomp.SetFromU(upperTri)
	choleskyDecomp.ToSym(covMatrix)
	ok := choleskyDecomp.Factorize(covMatrix)
	if !ok {
		panic("cholesky decomp for covariance matrix failed")
	}
	g.choleskyDecomp = choleskyDecomp
}

func (g *GaussianIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	diff := make([]float64, g.stateWidth)
	stateDiffVector := mat.NewVecDense(
		g.stateWidth,
		floats.SubTo(diff, currentState, pastState),
	)
	var vectorInvMat mat.VecDense
	err := g.choleskyDecomp.SolveVecTo(&vectorInvMat, stateDiffVector)
	if err != nil {
		return math.NaN()
	}
	logResult := -0.5 * mat.Dot(&vectorInvMat, stateDiffVector)
	logResult -= 0.5 * float64(g.stateWidth) * logTwoPi
	logResult -= 0.5 * g.choleskyDecomp.LogDet()
	return math.Exp(logResult)
}
