package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

const logTwoPi = 1.83788

// GaussianCovarianceKernel is an interface that must be implemented
// in order to create a covariance kernel that can be used in the
// GaussianIntegrationKernel.
type GaussianCovarianceKernel interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	SetParams(params *simulator.OtherParams)
	GetCovariance(
		currentState []float64,
		pastState []float64,
		currentTime float64,
		pastTime float64,
	) *mat.SymDense
}

// GaussianIntegrationKernel applies a Gaussian kernel to get a vector of means.
type GaussianIntegrationKernel struct {
	Kernel     GaussianCovarianceKernel
	means      []float64
	stateWidth int
}

func (g *GaussianIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Kernel.Configure(partitionIndex, settings)
	g.stateWidth = settings.StateWidths[partitionIndex]
	g.SetParams(settings.OtherParams[partitionIndex])
}

func (g *GaussianIntegrationKernel) SetParams(params *simulator.OtherParams) {
	g.means = params.FloatParams["means"]
	g.Kernel.SetParams(params)
}

func (g *GaussianIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	currentDiff := make([]float64, g.stateWidth)
	pastDiff := make([]float64, g.stateWidth)
	currentStateDiffVector := mat.NewVecDense(
		g.stateWidth,
		floats.SubTo(currentDiff, currentState, g.means),
	)
	pastStateDiffVector := mat.NewVecDense(
		g.stateWidth,
		floats.SubTo(pastDiff, pastState, g.means),
	)
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(
		g.Kernel.GetCovariance(currentState, pastState, currentTime, pastTime),
	)
	if !ok {
		return math.NaN()
	}
	var vectorInvMat mat.VecDense
	err := choleskyDecomp.SolveVecTo(&vectorInvMat, currentStateDiffVector)
	if err != nil {
		return math.NaN()
	}
	logResult := -0.5 * mat.Dot(&vectorInvMat, pastStateDiffVector)
	logResult -= 0.5 * float64(g.stateWidth) * logTwoPi
	logResult -= 0.5 * choleskyDecomp.LogDet()
	return math.Exp(logResult)
}
