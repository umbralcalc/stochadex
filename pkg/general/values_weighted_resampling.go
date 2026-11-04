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
//   - Each value in the history is drawn with probability proportional to
//     its own weight, read from the same history row of the paired
//     log-weight partition, times the discounting factor raised to the
//     row's depth. Rows the run has not yet reached are not drawn.
//
// Update:
//
//	(k, j) ~ Categorical(p), p(k, j) proportional to exp(L_j(k)) * past_discounting_factor^(k-1)
//	for k = 1..min(D, step) and j over log_weight_partitions, where
//	L_j(k) = log_weight_partitions[j]@k[log_weight_indices[j]] (element 0 if unset)
//	and D = log_weight_partitions[0]'s state_history_depth
//	x <- data_values_partitions[j]@k
type ValuesWeightedResamplingIteration struct {
	src     rand.Source
	catDist distuv.Categorical
}

// Configure implements simulator.Iteration.
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
	v.src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
	v.catDist = distuv.NewCategorical(nilWeights, v.src)
}

// Iterate implements simulator.Iteration.
func (v *ValuesWeightedResamplingIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logDiscount := math.Log(params.GetIndex("past_discounting_factor", 0))
	stateHistoryDepth := stateHistories[int(
		params.GetIndex("log_weight_partitions", 0))].StateHistoryDepth
	// At step k the history holds the k rows before this one; any deeper rows
	// are not yet filled, so they are given no weight.
	filled := min(stateHistoryDepth, timestepsHistory.CurrentStepNumber)
	logWeightPartitions := params.Get("log_weight_partitions")
	// Entry i*len(logWeightPartitions) + j is history row i of partition j.
	logWeights := make([]float64, 0, stateHistoryDepth*len(logWeightPartitions))
	for i := range stateHistoryDepth {
		for j, logWeightPartition := range logWeightPartitions {
			if i >= filled {
				logWeights = append(logWeights, math.Inf(-1))
				continue
			}
			var valueIndex int
			if v, ok := params.GetOk("log_weight_indices"); ok {
				valueIndex = int(v[j])
			} else {
				valueIndex = 0
			}
			logWeights = append(
				logWeights,
				stateHistories[int(logWeightPartition)].Values.At(i, valueIndex)+
					(logDiscount*float64(i)),
			)
		}
	}
	// Reweighting all at once, rather than one entry at a time, never passes
	// through a zero total weight on the way.
	logNorm := floats.LogSumExp(logWeights)
	for i, logWeight := range logWeights {
		logWeights[i] = math.Exp(logWeight - logNorm)
	}
	v.catDist.ReweightAll(logWeights)
	entry := int(v.catDist.Rand())
	dataPartition := params.GetIndex(
		"data_values_partitions", entry%len(logWeightPartitions))
	return stateHistories[int(dataPartition)].CopyStateRow(
		entry / len(logWeightPartitions))
}
