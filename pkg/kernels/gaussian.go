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
	g.SetParams(settings.Params[partitionIndex])
}

func (g *GaussianIntegrationKernel) SetParams(params simulator.Params) {
	g.stateWidth = int(math.Sqrt(float64(len(params["covariance_matrix"]))))
	covMatrix := mat.NewSymDense(g.stateWidth, params["covariance_matrix"])
	var choleskyDecomp mat.Cholesky
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
