package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// DataComparisonUpdateIteration allows for any log-likelihood update to be
// used in computing the update to params given input data.
type DataComparisonUpdateIteration struct {
	Likelihood LikelihoodDistributionWithUpdate
}

func (d *DataComparisonUpdateIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.Likelihood.SetSeed(partitionIndex, settings)
}

func (d *DataComparisonUpdateIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return d.Likelihood.EvaluateUpdate(
		params, partitionIndex, stateHistories, timestepsHistory)
}
