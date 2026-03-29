package inference

import (
	"math"
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
)

// NormalLikelihoodDistribution models data with a multivariate normal.
//
// Usage hints:
//   - Provide mean/covariance via params or upstream partition outputs.
//   - Optional "default_covariance" used if provided covariance is not PD
//     only when AllowDefaultCovarianceFallback is true (otherwise this
//     situation panics with an explicit message).
//   - Optional "cov_burn_in_steps": for outer step counts ≤ this value, the
//     covariance is taken from default_covariance when that param is set,
//     ignoring streamed covariance from upstream (fixed proposal / prior phase).
//     If default_covariance is absent during burn-in, behaviour falls back to
//     CovarianceMatrixFromParamsOrPartition as usual.
//   - For diagonal Gaussian proposals without full covariance conditioning,
//     supply "variance" (diagonal) or wire a variance partition instead of a
//     dense covariance_matrix upstream.
//   - GenerateNewSamples draws from the current parameterised distribution.
type NormalLikelihoodDistribution struct {
	Src        rand.Source
	mean       *mat.VecDense
	covariance *mat.SymDense
	defaultCov []float64
	// AllowDefaultCovarianceFallback must be true to substitute
	// default_covariance when the primary matrix is not positive-definite.
	AllowDefaultCovarianceFallback bool
}

func (n *NormalLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (n *NormalLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	n.mean = MeanFromParamsOrPartition(params, partitionIndex, stateHistories)
	burnK := 0
	if b, ok := params.GetOk("cov_burn_in_steps"); ok && timestepsHistory != nil {
		burnK = int(b[0])
	}
	inBurn := burnK > 0 && timestepsHistory != nil &&
		timestepsHistory.CurrentStepNumber <= burnK
	if inBurn {
		if dc, ok := params.GetOk("default_covariance"); ok {
			dim := int(math.Sqrt(float64(len(dc))))
			n.covariance = mat.NewSymDense(dim, dc)
		} else {
			n.covariance = CovarianceMatrixFromParamsOrPartition(
				params,
				partitionIndex,
				stateHistories,
			)
		}
	} else {
		n.covariance = CovarianceMatrixFromParamsOrPartition(
			params,
			partitionIndex,
			stateHistories,
		)
	}
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
			if !n.AllowDefaultCovarianceFallback {
				panic("inference.NormalLikelihoodDistribution: covariance not positive-definite; set AllowDefaultCovarianceFallback to use default_covariance or fix the streamed matrix")
			}
			dist, ok = distmv.NewNormal(
				n.mean.RawVector().Data,
				mat.NewSymDense(n.mean.Len(), n.defaultCov),
				n.Src,
			)
			if !ok {
				panic("inference.NormalLikelihoodDistribution: default_covariance is also not positive-definite")
			}
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
