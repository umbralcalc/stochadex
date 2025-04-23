package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// PosteriorLogNormalisationIteration updates the cumulative normalisation of the
// posterior distribution over params using log-likelihood values given in the
// state history of other partitions as well as a specified past discounting factor.
type PosteriorLogNormalisationIteration struct {
}

func (p *PosteriorLogNormalisationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (p *PosteriorLogNormalisationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logDiscount := math.Log(params.GetIndex("past_discounting_factor", 0))
	logLikes := make([]float64, len(params.Get("loglike_partitions")))
	stateHistoryDepth :=
		stateHistories[int(params.GetIndex("loglike_partitions", 0))].StateHistoryDepth
	logNorms := make([]float64, stateHistoryDepth)
	for i := range stateHistoryDepth {
		for j, loglikePartition := range params.Get("loglike_partitions") {
			var valueIndex int
			if v, ok := params.GetOk("loglike_indices"); ok {
				valueIndex = int(v[j])
			} else {
				valueIndex = 0
			}
			logLikes[j] = stateHistories[int(loglikePartition)].Values.At(i, valueIndex)
		}
		logNorms[i] = floats.LogSumExp(logLikes) + (logDiscount * float64(i))
	}
	return []float64{floats.LogSumExp(logNorms)}
}
