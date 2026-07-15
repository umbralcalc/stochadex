package inference

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmat"
)

// WishartLikelihoodDistribution assumes the real data are well described
// by a Wishart distribution, given the input degrees of freedom and scale
// matrix.
type WishartLikelihoodDistribution struct {
	Src          rand.Source
	dims         int
	dof          float64
	scale        *mat.SymDense
	defaultScale []float64
	// gradInvScale caches the inverse of the scale matrix for EvaluateLogLikeMeanGrad,
	// which the gradient iteration calls once per row of a data batch that all share this
	// scale. Without the cache each call re-factorises and re-inverts (both O(d^3)).
	// Invalidated in SetParams, recomputed lazily. Kept immutable across calls (the scaled
	// term below is built separately) so the cache survives the batch.
	gradInvScale mat.SymDense
	gradInvReady bool
}

func (w *WishartLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

func (w *WishartLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	w.dof = params.Get("degrees_of_freedom")[0]
	scale := params.Get("scale_matrix")
	w.dims = int(math.Sqrt(float64(len(scale))))
	w.scale = mat.NewSymDense(w.dims, scale)
	if s, ok := params.GetOk("default_scale"); ok {
		w.defaultScale = s
	}
	w.gradInvReady = false // scale may have changed; recompute the inverse lazily on next gradient
}

func (w *WishartLikelihoodDistribution) getDist() *distmat.Wishart {
	dist, ok := distmat.NewWishart(w.scale, w.dof, w.Src)
	if !ok {
		if w.defaultScale != nil {
			dist, _ = distmat.NewWishart(
				mat.NewSymDense(w.dims, w.defaultScale),
				w.dof,
				w.Src,
			)
		} else {
			panic("scale matrix is not positive-definite")
		}
	}
	return dist
}

func (w *WishartLikelihoodDistribution) EvaluateLogLike(data []float64) float64 {
	dist := w.getDist()
	return dist.LogProbSym(mat.NewSymDense(w.dims, data))
}

func (w *WishartLikelihoodDistribution) GenerateNewSamples() []float64 {
	dist := w.getDist()
	data := mat.NewSymDense(w.dims, nil)
	dist.RandSymTo(data)
	return data.RawSymmetric().Data
}

func (w *WishartLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	if !w.gradInvReady {
		var choleskyDecomp mat.Cholesky
		if ok := choleskyDecomp.Factorize(w.scale); !ok {
			panic("cholesky decomp for scale matrix failed")
		}
		w.gradInvScale.CopySym(w.scale)
		choleskyDecomp.InverseTo(&w.gradInvScale)
		w.gradInvReady = true
	}
	llg := make([]float64, len(data))
	copy(llg, data)
	logLikeGrad := mat.NewDense(w.dims, w.dims, llg)
	logLikeGrad.Mul(logLikeGrad, &w.gradInvScale)
	logLikeGrad.Mul(&w.gradInvScale, logLikeGrad)
	logLikeGrad.Scale(0.5, logLikeGrad)
	// Build the 0.5·dof·scale⁻¹ term separately so the cached inverse stays intact.
	var scaledInv mat.SymDense
	scaledInv.ScaleSym(0.5*float64(w.dof), &w.gradInvScale)
	logLikeGrad.Sub(logLikeGrad, &scaledInv)
	return logLikeGrad.RawMatrix().Data
}
