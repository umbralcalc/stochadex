package general

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
	"gonum.org/v1/gonum/stat/distuv"
)

// WeightedResamplingIteration resamples from the history of state values
// of other partitions with optional frequencies according to the provided
// weights and optional additional noise applied to each sample.
type WeightedResamplingIteration struct {
	Src     rand.Source
	catDist distuv.Categorical
}

func (w *WeightedResamplingIteration) Configure(
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
	w.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	w.catDist = distuv.NewCategorical(nilWeights, w.Src)
}

func (w *WeightedResamplingIteration) Iterate(
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
		w.catDist.Reweight(i, math.Exp(logWeight-logNorm))
	}
	indexPair := indices[int(w.catDist.Rand())]
	dataPartition := params.GetIndex("data_values_partitions", indexPair[1])
	sampleCentre := stateHistories[int(dataPartition)].Values.RawRowView(indexPair[0])
	if sampleCov, ok := params.GetOk("noise_covariance"); ok {
		normDist, ok := distmv.NewNormal(
			sampleCentre,
			mat.NewSymDense(len(sampleCentre), sampleCov),
			w.Src,
		)
		if !ok {
			panic("covariance matrix is not positive-definite")
		}
		return normDist.Rand(nil)
	} else {
		return sampleCentre
	}
}
