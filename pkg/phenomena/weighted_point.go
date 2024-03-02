package phenomena

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// WeightedPointIteration computed the weighted sample average for each state vector
// element across all of the neighbouring partitions.
type WeightedPointIteration struct {
}

func (w *WeightedPointIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (w *WeightedPointIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	scaledVec := mat.NewVecDense(stateHistories[partitionIndex].StateWidth, nil)
	scaledVec.ScaleVec(
		params.FloatParams["neighbour_weightings"][0],
		stateHistories[params.IntParams["neighbour_partitions"][0]].Values.RowView(0),
	)
	latestFieldValues := scaledVec
	normalisation := params.FloatParams["neighbour_weightings"][0]
	for i, index := range params.IntParams["neighbour_partitions"] {
		if i == 0 {
			continue
		}
		scaledVec.ScaleVec(
			params.FloatParams["neighbour_weightings"][i],
			stateHistories[index].Values.RowView(0),
		)
		latestFieldValues.AddVec(
			latestFieldValues,
			scaledVec,
		)
		normalisation += params.FloatParams["neighbour_weightings"][i]
	}
	latestFieldValues.ScaleVec(1.0/normalisation, latestFieldValues)
	return latestFieldValues.RawVector().Data
}
