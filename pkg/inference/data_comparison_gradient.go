package inference

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// LikelihoodDistributionGradient is the interface that must be implemented in
// order to create a likelihood which computes a gradient.
type LikelihoodDistributionGradient interface {
	LikelihoodDistribution
	EvaluateLogLikeMeanGrad(
		mean *mat.VecDense,
		covariance mat.Symmetric,
		data []float64,
	) []float64
}

// MeanGradientFunc computes the gradient with respect to the mean directly.
func MeanGradientFunc(
	params *simulator.Params,
	likeMeanGrad []float64,
) []float64 {
	return likeMeanGrad
}

// DataComparisonGradientIteration allows for any log-likelihood gradient to be
// used in computing the overall comparison distribution gradient.
type DataComparisonGradientIteration struct {
	Likelihood   LikelihoodDistributionGradient
	GradientFunc func(
		params *simulator.Params,
		likeMeanGrad []float64,
	) []float64
	Batch *simulator.StateHistory
}

func (d *DataComparisonGradientIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.Likelihood.Configure(partitionIndex, settings)
}

func (d *DataComparisonGradientIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	mean := make([]float64, stateHistories[partitionIndex].StateWidth)
	copy(mean, stateHistories[int(
		params.Get("mean_partition")[0])].Values.RawRowView(0))
	dims := len(mean)
	var covMat *mat.SymDense
	cVals, ok := params.GetOk("covariance_matrix")
	if ok {
		covMat = mat.NewSymDense(dims, cVals)
	} else if varVals, ok := params.GetOk("variance"); ok {
		cVals = make([]float64, 0)
		for i := range dims {
			for j := range dims {
				switch i {
				case j:
					cVals = append(cVals, varVals[i])
				default:
					cVals = append(cVals, 0.0)
				}
			}
		}
		covMat = mat.NewSymDense(dims, cVals)
	}
	likeMeanGrad := make([]float64, len(mean))
	meanVec := mat.NewVecDense(dims, mean)
	for i := range d.Batch.StateHistoryDepth {
		floats.Add(likeMeanGrad, d.Likelihood.EvaluateLogLikeMeanGrad(
			meanVec,
			covMat,
			d.Batch.Values.RawRowView(i),
		))
	}
	return d.GradientFunc(params, likeMeanGrad)
}

func (d *DataComparisonGradientIteration) UpdateMemory(
	params *simulator.Params,
	update *general.StateMemoryUpdate,
) {
	if _, ok := params.GetOk(update.Name + "->data_values"); ok {
		d.Batch = update.StateHistory
	} else {
		panic("data comparison gradient: memory update from partition: " +
			update.Name + " has no configured use")
	}
}
