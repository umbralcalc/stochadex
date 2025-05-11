package inference

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
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

func (w *WishartLikelihoodDistribution) SetSeed(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
}

// TODO: Add short-circuit to update params from packet
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

// TODO: Fix this to add log weights
func (w *WishartLikelihoodDistribution) EvaluateUpdate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	latestStateValues := params.Get("latest_data_values")
	discount := params.Get("past_discounting_factor")[0]
	w.dof *= discount
	w.scale.ScaleSym(discount, w.scale)
	w.dof = w.dof + 1.0
	w.scale.SymRankOne(
		w.scale,
		1.0,
		mat.NewVecDense(
			len(latestStateValues),
			floats.SubTo(
				make([]float64, len(latestStateValues)),
				latestStateValues,
				stateHistories[int(params.GetIndex(
					"data_values_partition", 0))].Values.RawRowView(0),
			),
		),
	)
	return append(w.scale.RawSymmetric().Data, w.dof)
}
