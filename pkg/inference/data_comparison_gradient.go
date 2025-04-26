package inference

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

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
	Likelihood   LikelihoodDistributionWithGradient
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
	d.Likelihood.SetParams(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	likeMeanGrad := make([]float64, stateHistories[partitionIndex].StateWidth)
	for i := range d.Batch.StateHistoryDepth {
		floats.Add(likeMeanGrad, d.Likelihood.EvaluateLogLikeMeanGrad(
			d.Batch.Values.RawRowView(i),
		))
	}
	floats.Scale(1.0/float64(d.Batch.StateHistoryDepth), likeMeanGrad)
	return d.GradientFunc(params, likeMeanGrad)
}

func (d *DataComparisonGradientIteration) UpdateMemory(
	params *simulator.Params,
	update general.StateMemoryUpdate,
) {
	if _, ok := params.GetOk(update.Name + "->data_values"); ok {
		d.Batch = update.StateHistory
	} else {
		panic("data comparison gradient: memory update from partition: " +
			update.Name + " has no configured use")
	}
}
