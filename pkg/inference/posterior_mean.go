package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// VarianceTransform returns the values needed to compute the variance.
func VarianceTransform(
	params *simulator.Params,
	values mat.Vector,
) mat.Vector {
	diff := mat.NewVecDense(values.Len(), nil)
	diff.SubVec(
		mat.NewVecDense(values.Len(), params.Get("mean")),
		values,
	)
	diff.MulElemVec(diff, diff)
	return diff
}

// MeanTransform returns the values needed to compute the mean.
func MeanTransform(
	params *simulator.Params,
	values mat.Vector,
) mat.Vector {
	return values
}

// PosteriorMeanIteration updates an estimate of the mean of the posterior
// distribution over params using log-likelihood and param values given in
// the state history of other partitions.
//
// Update:
//
//	l_j = loglike_partitions[j]@1[loglike_indices[j]]       (index 0 when loglike_indices is unset)
//	c = posterior_log_normalisation
//	T = log(exp(c) + sum_j exp(l_j))
//	x <- exp(c - T) x@1 + sum_j exp(l_j - T) transform(param_partitions[j]@1)
//	x <- x@1 if T is NaN or infinite; a non-finite exp(c - T) counts as 0
//	and a term whose exp(l_j - T) is non-finite is skipped.
//	transform(v) is v for mean, and (mean - v)^2 per element for variance.
type PosteriorMeanIteration struct {
	Transform func(
		params *simulator.Params,
		values mat.Vector,
	) mat.Vector
}

// Configure implements simulator.Iteration.
func (p *PosteriorMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

// Iterate implements simulator.Iteration.
func (p *PosteriorMeanIteration) Iterate(
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
	mean := mat.VecDenseCopyOf(stateHistories[partitionIndex].Values.RowView(0))
	scalePast := math.Exp(logNormPast - logNormTotal)
	if math.IsNaN(scalePast) || math.IsInf(scalePast, 0) {
		scalePast = 0
	}
	mean.ScaleVec(scalePast, mean)
	for i, paramsPartition := range params.Get("param_partitions") {
		w := math.Exp(logLikes[i] - logNormTotal)
		if math.IsNaN(w) || math.IsInf(w, 0) {
			continue
		}
		mean.AddScaledVec(
			mean,
			w,
			p.Transform(
				params,
				stateHistories[int(paramsPartition)].Values.RowView(0),
			),
		)
	}
	return mean.RawVector().Data
}
