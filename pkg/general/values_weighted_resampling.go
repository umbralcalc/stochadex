package general

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat/distuv"
)

// ValuesWeightedResamplingIteration resamples historical values from other
// partitions according to provided (optionally discounted) weights.
//
// Usage hints:
//   - Provide: "log_weight_partitions" (and optional "log_weight_indices").
//   - Provide: "data_values_partitions" to choose which values to resample.
//   - Use "past_discounting_factor" to downweight older history (exponential).
type ValuesWeightedResamplingIteration struct {
	Src     rand.Source
	catDist distuv.Categorical
}

func (v *ValuesWeightedResamplingIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	logWeightPartitions :=
		settings.Iterations[partitionIndex].Params.Get("log_weight_partitions")
	nilWeights := make(
		[]float64,
		len(logWeightPartitions)*
			settings.Iterations[int(logWeightPartitions[0])].StateHistoryDepth,
	)
	nilWeights[0] = 1.0
	v.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
	v.catDist = distuv.NewCategorical(nilWeights, v.Src)
}

func (v *ValuesWeightedResamplingIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logDiscount := math.Log(params.GetIndex("past_discounting_factor", 0))
	stateHistoryDepth := stateHistories[int(
		params.GetIndex("log_weight_partitions", 0))].StateHistoryDepth
	logWeights := make([]float64, 0)
	indices := make([][]int, 0)
	for i := range stateHistoryDepth {
		for j, logWeightPartition := range params.Get("log_weight_partitions") {
			var valueIndex int
			if v, ok := params.GetOk("log_weight_indices"); ok {
				valueIndex = int(v[j])
			} else {
				valueIndex = 0
			}
			logWeights = append(
				logWeights,
				stateHistories[int(logWeightPartition)].Values.At(0, valueIndex)+
					(logDiscount*float64(i)),
			)
			indices = append(indices, []int{i, j})
		}
	}
	logNorm := floats.LogSumExp(logWeights)
	for i, logWeight := range logWeights {
		v.catDist.Reweight(i, math.Exp(logWeight-logNorm))
	}
	indexPair := indices[int(v.catDist.Rand())]
	dataPartition := params.GetIndex("data_values_partitions", indexPair[1])
	return stateHistories[int(dataPartition)].CopyStateRow(indexPair[0])
}
