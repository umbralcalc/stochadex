package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ValuesSortedCollectionMeanIteration computes a weighted mean of the top
// entries in a sorted collection, blended with its own previous state via
// a learning rate. This enables adaptive mean updates for evolution
// strategies or similar rank-based optimisation algorithms.
//
// Usage hints:
//   - Provide: "sorted_collection" (flattened sorted state from upstream),
//     "weights" (rank-based weights for top-μ entries, should sum to 1),
//     "learning_rate" (blend factor α), and "values_state_width" (dimension D
//     of each entry, excluding sort key).
//   - Sorted collection entries are ordered highest sort key first (index 0).
type ValuesSortedCollectionMeanIteration struct {
}

func (v *ValuesSortedCollectionMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesSortedCollectionMeanIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	sortedCollection := params.Get("sorted_collection")
	weights := params.Get("weights")
	learningRate := params.GetIndex("learning_rate", 0)
	valuesWidth := int(params.GetIndex("values_state_width", 0))
	entryWidth := valuesWidth + 1

	previousMean := stateHistories[partitionIndex].Values.RawRowView(0)
	newMean := make([]float64, valuesWidth)

	for i := range len(weights) {
		offset := i*entryWidth + 1
		for j := range valuesWidth {
			newMean[j] += weights[i] * sortedCollection[offset+j]
		}
	}

	for j := range newMean {
		newMean[j] = (1-learningRate)*previousMean[j] + learningRate*newMean[j]
	}

	return newMean
}
