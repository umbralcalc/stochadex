package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// PosteriorCovarianceIteration updates an estimate of the covariance matrix
// of the posterior distribution over params using log-likelihood and param
// values given in the state history of other partitions, and a mean vector.
//
// Usage hints:
//   - The state is the full d×d covariance matrix flattened row-major, where
//     d is the length of "mean". The update is computed on its upper triangle
//     (as mat.SymDense reads it), and the lower triangle mirrors it, so the
//     state reads the same as a full matrix or as mat.SymDense data.
//
// Update:
//
//	l_j = loglike_partitions[j]@1[loglike_indices[j]]       (index 0 when loglike_indices is unset)
//	c = posterior_log_normalisation
//	T = log(exp(c) + sum_j exp(l_j))
//	e_j = mean - param_partitions[j]@1
//	C <- exp(c - T) C@1 + sum_j exp(l_j - T) e_j e_j^T
//	x is C, d x d row-major with d the length of mean. The upper triangle
//	(x[i*d + k], k >= i) is updated from that of x@1, and the lower triangle
//	mirrors it: x[k*d + i] = x[i*d + k].
//	x <- x@1 if T is NaN or infinite; a non-finite exp(c - T) counts as 0
//	and a term whose exp(l_j - T) is non-finite is skipped.
type PosteriorCovarianceIteration struct {
}

// Configure implements simulator.Iteration.
func (p *PosteriorCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

// Iterate implements simulator.Iteration.
func (p *PosteriorCovarianceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logLikes := make([]float64, 0)
	for i, loglikePartition := range params.Get("loglike_partitions") {
		var valueIndex int
		if v, ok := params.GetOk("loglike_indices"); ok {
			valueIndex = int(v[i])
		} else {
			valueIndex = 0
		}
		logLikes = append(
			logLikes,
			stateHistories[int(loglikePartition)].Values.At(0, valueIndex),
		)
	}
	logNormLatest := floats.LogSumExp(logLikes)
	logNormPast := params.GetIndex("posterior_log_normalisation", 0)
	logNormTotal := floats.LogSumExp([]float64{logNormLatest, logNormPast})
	if math.IsNaN(logNormTotal) || math.IsInf(logNormTotal, 0) {
		return stateHistories[partitionIndex].CopyStateRow(0)
	}
	dims := len(params.Get("mean"))
	covMatValues := stateHistories[partitionIndex].CopyStateRow(0)
	covMat := mat.NewSymDense(dims, covMatValues)
	scalePast := math.Exp(logNormPast - logNormTotal)
	if math.IsNaN(scalePast) || math.IsInf(scalePast, 0) {
		scalePast = 0
	}
	covMat.ScaleSym(scalePast, covMat)
	mean := mat.NewVecDense(dims, params.Get("mean"))
	diffs := mat.NewVecDense(dims, nil)
	for i, paramsPartition := range params.Get("param_partitions") {
		w := math.Exp(logLikes[i] - logNormTotal)
		if math.IsNaN(w) || math.IsInf(w, 0) {
			continue
		}
		diffs.SubVec(mean, stateHistories[int(paramsPartition)].Values.RowView(0))
		covMat.SymRankOne(covMat, w, diffs)
	}
	// mat.SymDense updates only the upper triangle, so mirror it into the
	// lower one to keep the whole state symmetric.
	for i := range dims {
		for j := range i {
			covMatValues[i*dims+j] = covMatValues[j*dims+i]
		}
	}
	return covMatValues
}
