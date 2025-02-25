package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// SquaredExponentialStateIntegrationKernel applies a Gaussian kernel using
// an input target state vector and covariance as input.
type SquaredExponentialStateIntegrationKernel struct {
	choleskyDecomp mat.Cholesky
	targetState    []float64
	stateWidth     int
}

func (s *SquaredExponentialStateIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	s.SetParams(&settings.Iterations[partitionIndex].Params)
}

func (s *SquaredExponentialStateIntegrationKernel) SetParams(params *simulator.Params) {
	s.targetState = params.Get("target_state")
	s.stateWidth = len(s.targetState)
	covMatrix := mat.NewSymDense(s.stateWidth, params.Get("covariance_matrix"))
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(covMatrix)
	if !ok {
		panic("cholesky decomp for covariance matrix failed")
	}
	s.choleskyDecomp = choleskyDecomp
}

func (s *SquaredExponentialStateIntegrationKernel) Evaluate(
	currentState []float64,
	pastState []float64,
	currentTime float64,
	pastTime float64,
) float64 {
	currentDiff := make([]float64, s.stateWidth)
	pastDiff := make([]float64, s.stateWidth)
	currentStateDiffVector := mat.NewVecDense(
		s.stateWidth,
		floats.SubTo(currentDiff, currentState, s.targetState),
	)
	pastStateDiffVector := mat.NewVecDense(
		s.stateWidth,
		floats.SubTo(pastDiff, pastState, s.targetState),
	)
	var vectorInvMat mat.VecDense
	err := s.choleskyDecomp.SolveVecTo(&vectorInvMat, currentStateDiffVector)
	if err != nil {
		return math.NaN()
	}
	return math.Exp(-0.5 * mat.Dot(&vectorInvMat, pastStateDiffVector))
}
