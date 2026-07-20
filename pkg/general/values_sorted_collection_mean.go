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

	// Skip unfilled collection slots (sort-by == empty_value) so the sentinel
	// never enters the weighted mean before the collection fills; renormalise the
	// weights over the filled entries. Absent empty_value this is a no-op (the
	// weights are used as given). If nothing is filled yet, hold the previous mean.
	emptyValue, hasEmpty := params.GetOk("empty_value")
	totalWeight := 0.0
	for i := range len(weights) {
		base := i * entryWidth
		if hasEmpty && sortedCollection[base] == emptyValue[0] {
			continue
		}
		for j := range valuesWidth {
			newMean[j] += weights[i] * sortedCollection[base+1+j]
		}
		totalWeight += weights[i]
	}
	if hasEmpty {
		if totalWeight == 0 {
			return stateHistories[partitionIndex].CopyStateRow(0)
		}
		for j := range newMean {
			newMean[j] /= totalWeight
		}
	}

	for j := range newMean {
		newMean[j] = (1-learningRate)*previousMean[j] + learningRate*newMean[j]
	}

	return newMean
}
