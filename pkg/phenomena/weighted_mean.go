package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// WeightedMeanIteration computed the weighted sample average for each state vector
// element across all of the neighbouring partitions.
type WeightedMeanIteration struct {
}

func (w *WeightedMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (w *WeightedMeanIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	scaledVec := mat.NewVecDense(stateHistories[partitionIndex].StateWidth, nil)
	scaledVec.ScaleVec(
		params["neighbour_weightings"][0],
		stateHistories[int(params["neighbour_partitions"][0])].Values.RowView(0),
	)
	latestFieldValues := scaledVec
	normalisation := params["neighbour_weightings"][0]
	for i, index := range params["neighbour_partitions"] {
		if i == 0 {
			continue
		}
		scaledVec.ScaleVec(
			params["neighbour_weightings"][i],
			stateHistories[int(index)].Values.RowView(0),
		)
		latestFieldValues.AddVec(
			latestFieldValues,
			scaledVec,
		)
		normalisation += params["neighbour_weightings"][i]
	}
	latestFieldValues.ScaleVec(1.0/normalisation, latestFieldValues)
	return latestFieldValues.RawVector().Data
}
