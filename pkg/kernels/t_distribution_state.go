package kernels

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// TDistributionStateIntegrationKernel applies a t-distribution kenel using
// an input scale matrix and degrees of freedom parameters.
type TDistributionStateIntegrationKernel struct {
	choleskyDecomp mat.Cholesky
	determinant    float64
	dof            float64
}

func (t *TDistributionStateIntegrationKernel) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (t *TDistributionStateIntegrationKernel) SetParams(params *simulator.Params) {
	scaleParams := params.Get("scale_matrix")
	covMatrix := mat.NewSymDense(int(math.Sqrt(float64(len(scaleParams)))), scaleParams)
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(covMatrix)
	if !ok {
		panic("cholesky decomp for scale matrix failed")
	}
	t.choleskyDecomp = choleskyDecomp
	t.determinant = t.choleskyDecomp.Det()
	t.dof = params.GetIndex("degrees_of_freedom", 0)
}

func (t *TDistributionStateIntegrationKernel) Evaluate(
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
	err := t.choleskyDecomp.SolveVecTo(vectorInvMat, diffVector)
	if err != nil {
		panic(err)
	}
	return math.Pow(
		1.0+(mat.Dot(vectorInvMat, diffVector)/t.dof),
		-0.5*(float64(stateWidth)+t.dof),
	) / t.determinant
}
