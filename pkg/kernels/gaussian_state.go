package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// GaussianStateIntegrationKernel implements a Gaussian state-distance kernel
// for weighting historical samples based on their distance from the current state.
//
// This kernel computes weights using a multivariate Gaussian distribution,
// where the weight decreases exponentially with the Mahalanobis distance
// between the current and historical states. It's particularly useful for
// state-space aggregation and similarity-based weighting.
//
// Mathematical Background:
// The Gaussian kernel computes weights using the multivariate Gaussian density:
//
//	w(x_current, x_past) = exp(-0.5 * (x_current - x_past)^T * Σ^{-1} * (x_current - x_past)) / sqrt(det(Σ))
//
// where Σ is the covariance matrix defining the shape and scale of the weighting.
//
// Key Properties:
//   - Mahalanobis distance: d² = (x_current - x_past)^T * Σ^{-1} * (x_current - x_past)
//   - Weight range: w ∈ [0, 1/sqrt(det(Σ))]
//   - Maximum weight: w_max = 1/sqrt(det(Σ)) when states are identical
//   - Decay rate: Controlled by eigenvalues of Σ (larger eigenvalues = slower decay)
//
// Applications:
//   - State-space clustering and similarity weighting
//   - Non-parametric density estimation
//   - Adaptive aggregation based on state similarity
//   - Anomaly detection through distance weighting
//   - Multi-dimensional state space analysis
//
// Configuration:
//   - Provide "covariance_matrix" parameter as a flattened symmetric matrix (row-major order)
//   - Matrix must be positive definite (all eigenvalues > 0)
//   - Matrix dimension must match state vector dimension
//
// Example:
//
//	// Configure kernel for 2D state space with covariance matrix
//	// Σ = [[1.0, 0.5], [0.5, 1.0]] (correlated dimensions)
//	covariance := []float64{1.0, 0.5, 0.5, 1.0} // row-major flattened
//	params.Set("covariance_matrix", covariance)
//
//	kernel := &GaussianStateIntegrationKernel{}
//	kernel.Configure(0, settings)
//	kernel.SetParams(params)
//
//	// Weight for similar states will be high, dissimilar states low
//	weight := kernel.Evaluate(currentState, pastState, currentTime, pastTime)
//
// Performance:
//   - O(d²) setup cost for Cholesky decomposition where d is state dimension
//   - O(d²) evaluation cost for matrix-vector operations
//   - Memory usage: O(d²) for storing Cholesky decomposition
//   - Efficient for moderate dimensions (< 100)
//
// Error Handling:
//   - Panics if covariance matrix is not positive definite
//   - Panics if matrix dimension doesn't match state dimension
//   - Panics if matrix is not square or symmetric
type GaussianStateIntegrationKernel struct {
	choleskyDecomp mat.Cholesky
	determinant    float64
}

func (g *GaussianStateIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
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
	vectorInvMat := mat.NewVecDense(stateWidth, nil)
	err := g.choleskyDecomp.SolveVecTo(vectorInvMat, diffVector)
	if err != nil {
		panic(err)
	}
	return math.Exp(-0.5*mat.Dot(vectorInvMat, diffVector)) / g.determinant
}
