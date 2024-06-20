package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

const logTwoPi = 1.83788

// GaussianProcessCovarianceKernel is an interface that must be
// implemented in order to create a covariance kernel that can
// be used in the GaussianProcessIntegrationKernel.
type GaussianProcessCovarianceKernel interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	SetParams(params *simulator.OtherParams)
	GetCovariance(
		currentState []float64,
		pastState []float64,
		currentTime float64,
		pastTime float64,
	) *mat.SymDense
}

// GaussianProcessIntegrationKernel applies a Gaussian kernel using
// an input target state vector and covariance kernel as input.
type GaussianProcessIntegrationKernel struct {
	Covariance             GaussianProcessCovarianceKernel
	currentCovarianceState []float64
	pastCovarianceState    []float64
	targetState            []float64
	stateWidth             int
}

func (g *GaussianProcessIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	g.Covariance.Configure(partitionIndex, settings)
	g.stateWidth = settings.StateWidths[partitionIndex]
	g.SetParams(settings.OtherParams[partitionIndex])
}

func (g *GaussianProcessIntegrationKernel) SetParams(params *simulator.OtherParams) {
	g.targetState = params.FloatParams["target_state"]
	g.currentCovarianceState = params.FloatParams["current_covariance_state"]
	g.pastCovarianceState = params.FloatParams["past_covariance_state"]
	g.Covariance.SetParams(params)
}

func (g *GaussianProcessIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	currentDiff := make([]float64, g.stateWidth)
	pastDiff := make([]float64, g.stateWidth)
	currentStateDiffVector := mat.NewVecDense(
		g.stateWidth,
		floats.SubTo(currentDiff, currentState, g.targetState),
	)
	pastStateDiffVector := mat.NewVecDense(
		g.stateWidth,
		floats.SubTo(pastDiff, pastState, g.targetState),
	)
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(
		g.Covariance.GetCovariance(
			g.currentCovarianceState,
			g.pastCovarianceState,
			currentTime,
			pastTime,
		),
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
