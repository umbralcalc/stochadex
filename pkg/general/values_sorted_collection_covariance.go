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
	mean := params.Get("mean")
	entryWidth := valuesWidth + 1

	newCov := mat.NewSymDense(valuesWidth, nil)
	diff := mat.NewVecDense(valuesWidth, nil)

	for i := range len(weights) {
		offset := i*entryWidth + 1
		for j := range valuesWidth {
			diff.SetVec(j, sortedCollection[offset+j]-mean[j])
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
