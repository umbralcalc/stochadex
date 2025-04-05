package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// GaussianStateIntegrationKernel applies a Gaussian kernel using
// an input covariance.
type GaussianStateIntegrationKernel struct {
	choleskyDecomp mat.Cholesky
	determinant    float64
}

func (g *GaussianStateIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.SetParams(&settings.Iterations[partitionIndex].Params)
}

func (g *GaussianStateIntegrationKernel) SetParams(params *simulator.Params) {
	covParams := params.Get("covariance_matrix")
	covMatrix := mat.NewSymDense(int(math.Sqrt(float64(len(covParams)))), covParams)
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(covMatrix)
	if !ok {
		panic("cholesky decomp for covariance matrix failed")
	}
	g.choleskyDecomp = choleskyDecomp
	g.determinant = g.choleskyDecomp.Det()
}

func (g *GaussianStateIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	stateWidth := len(currentState)
	diffVector := mat.NewVecDense(
		stateWidth,
		floats.SubTo(make([]float64, stateWidth), currentState, pastState),
	)
	var vectorInvMat mat.VecDense
	err := g.choleskyDecomp.SolveVecTo(&vectorInvMat, diffVector)
	if err != nil {
		panic(err)
	}
	return math.Exp(-0.5*mat.Dot(&vectorInvMat, diffVector)) / g.determinant
}
