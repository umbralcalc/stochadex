package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
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
}

func (w *WishartLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
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
	var choleskyDecomp mat.Cholesky
	ok := choleskyDecomp.Factorize(w.scale)
	if !ok {
		panic("cholesky decomp for scale matrix failed")
	}
	var invScale mat.SymDense
	invScale.CopySym(w.scale)
	choleskyDecomp.InverseTo(&invScale)
	llg := make([]float64, len(data))
	copy(llg, data)
	logLikeGrad := mat.NewDense(w.dims, w.dims, llg)
	logLikeGrad.Mul(logLikeGrad, &invScale)
	logLikeGrad.Mul(&invScale, logLikeGrad)
	logLikeGrad.Scale(0.5, logLikeGrad)
	invScale.ScaleSym(0.5*float64(w.dof), &invScale)
	logLikeGrad.Sub(logLikeGrad, &invScale)
	return logLikeGrad.RawMatrix().Data
}
