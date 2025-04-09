package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
)

// NormalLikelihoodDistribution assumes the real data are well described
// by a normal distribution, given the input mean and covariance matrix.
type NormalLikelihoodDistribution struct {
	Src        rand.Source
	mean       *mat.VecDense
	covariance *mat.SymDense
	defaultCov []float64
}

func (n *NormalLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	n.SetParams(&settings.Iterations[partitionIndex].Params)
}

func (n *NormalLikelihoodDistribution) SetParams(
	params *simulator.Params,
) {
	n.mean = MeanFromParams(params)
	n.covariance = CovarianceMatrixFromParams(params)
	if c, ok := params.GetOk("default_covariance"); ok {
		n.defaultCov = c
	}
}

func (n *NormalLikelihoodDistribution) getDist() *distmv.Normal {
	dist, ok := distmv.NewNormal(
		n.mean.RawVector().Data,
		n.covariance,
		n.Src,
	)
	if !ok {
		if n.defaultCov != nil {
			dist, _ = distmv.NewNormal(
				n.mean.RawVector().Data,
				mat.NewSymDense(n.mean.Len(), n.defaultCov),
				n.Src,
			)
		} else {
			panic("covariance matrix is not positive-definite")
		}
	}
	return dist
}

func (n *NormalLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := n.getDist()
	return dist.LogProb(data)
}

func (n *NormalLikelihoodDistribution) GenerateNewSamples() []float64 {
	dist := n.getDist()
	return dist.Rand(nil)
}

func (n *NormalLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	stateWidth := n.mean.Len()
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(n.covariance)
	if !ok {
		panic("cholesky decomp for covariance matrix failed")
	}
	logLikeGrad := mat.NewVecDense(stateWidth, nil)
	diffVector := mat.NewVecDense(
		stateWidth,
		floats.SubTo(make([]float64, stateWidth), data, n.mean.RawVector().Data),
	)
	err := choleskyDecomp.SolveVecTo(logLikeGrad, diffVector)
	if err != nil {
		panic(err)
	}
	return logLikeGrad.RawVector().Data
}
