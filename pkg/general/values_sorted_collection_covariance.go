package general

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// ValuesSortedCollectionCovarianceIteration computes a weighted covariance
// matrix of the top entries in a sorted collection around a provided mean,
// blended with its own previous state via a learning rate. This enables
// adaptive covariance updates for evolution strategies or similar rank-based
// optimisation algorithms.
//
// Usage hints:
//   - Provide: "sorted_collection" (flattened sorted state from upstream),
//     "weights" (rank-based weights for top-μ entries, should sum to 1),
//     "learning_rate" (blend factor α), "values_state_width" (dimension D
//     of each entry, excluding sort key), and "mean" (current mean vector
//     from the mean update partition).
//   - Output is a D×D flattened symmetric covariance matrix, consistent
//     with mat.SymDense.RawSymmetric().Data format.
type ValuesSortedCollectionCovarianceIteration struct {
}

func (v *ValuesSortedCollectionCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (v *ValuesSortedCollectionCovarianceIteration) Iterate(
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

	// Centre the covariance on the elite weighted mean (rank-µ), not on an
	// externally-supplied mean. Centring on a lagging blended mean folds the
	// per-step mean shift into the covariance as spurious variance, which
	// compounds and diverges (the estimate runs away to 1e6+). The spread of the
	// elites around their own centroid is the search width that must shrink as the
	// collection concentrates on the optimum, so the covariance contracts and the
	// search converges instead of exploding.
	eliteMean := make([]float64, valuesWidth)
	for i := range len(weights) {
		offset := i*entryWidth + 1
		for j := range valuesWidth {
			eliteMean[j] += weights[i] * sortedCollection[offset+j]
		}
	}

	newCov := mat.NewSymDense(valuesWidth, nil)
	diff := mat.NewVecDense(valuesWidth, nil)

	for i := range len(weights) {
		offset := i*entryWidth + 1
		for j := range valuesWidth {
			diff.SetVec(j, sortedCollection[offset+j]-eliteMean[j])
		}
		newCov.SymRankOne(newCov, weights[i], diff)
	}

	previousCov := stateHistories[partitionIndex].Values.RawRowView(0)
	covData := newCov.RawSymmetric().Data
	for j := range covData {
		covData[j] = (1-learningRate)*previousCov[j] + learningRate*covData[j]
	}

	return covData
}
