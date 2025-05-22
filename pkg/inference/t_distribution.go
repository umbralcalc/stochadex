package inference

import (
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
)

// TLikelihoodDistribution assumes the real data are well described by a
// Student's t-distribution, given the input degrees of freedom,
// mean and covariance matrix.
type TLikelihoodDistribution struct {
	Src        rand.Source
	dof        float64
	mean       *mat.VecDense
	covariance *mat.SymDense
	defaultCov []float64
}

func (t *TLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	t.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (t *TLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	t.dof = params.Get("degrees_of_freedom")[0]
	t.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	t.covariance = CovarianceMatrixFromParamsOrPartition(
		params,
		partitionIndex,
		stateHistories,
	)
	if c, ok := params.GetOk("default_covariance"); ok {
		t.defaultCov = c
	}
}

func (t *TLikelihoodDistribution) getDist() *distmv.StudentsT {
	dist, ok := distmv.NewStudentsT(
		t.mean.RawVector().Data,
		t.covariance,
		t.dof,
		t.Src,
	)
	if !ok {
		if t.defaultCov != nil {
			dist, _ = distmv.NewStudentsT(
				t.mean.RawVector().Data,
				mat.NewSymDense(t.mean.Len(), t.defaultCov),
				t.dof,
				t.Src,
			)
		} else {
			panic("covariance matrix is not positive-definite")
		}
	}
	return dist
}

func (t *TLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := t.getDist()
	return dist.LogProb(data)
}

func (t *TLikelihoodDistribution) GenerateNewSamples() []float64 {
	dist := t.getDist()
	return dist.Rand(nil)
}

func (t *TLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	stateWidth := t.mean.Len()
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(t.covariance)
	if !ok {
		panic("cholesky decomp for covariance matrix failed")
	}
	logLikeGrad := mat.NewVecDense(stateWidth, nil)
	diffVector := mat.NewVecDense(
		stateWidth,
		floats.SubTo(make([]float64, stateWidth), data, t.mean.RawVector().Data),
	)
	err := choleskyDecomp.SolveVecTo(logLikeGrad, diffVector)
	if err != nil {
		panic(err)
	}
	logLikeGrad.ScaleVec(
		0.5*(t.dof+float64(stateWidth))/(t.dof+mat.Dot(logLikeGrad, diffVector)),
		logLikeGrad,
	)
	return logLikeGrad.RawVector().Data
}
