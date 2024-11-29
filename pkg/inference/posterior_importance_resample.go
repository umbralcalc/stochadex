package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
	"gonum.org/v1/gonum/stat/distuv"
)

// PosteriorImportanceResampleIteration updates a sample frpm the posterior
// distribution over params using log-likelihood and param values given in
// the state history of other partitions.
type PosteriorImportanceResampleIteration struct {
	Src     rand.Source
	catDist distuv.Categorical
}

func (p *PosteriorImportanceResampleIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	loglikePartitions :=
		settings.Iterations[partitionIndex].Params.Get("loglike_partitions")
	nilWeights := make(
		[]float64,
		len(loglikePartitions)*
			settings.Iterations[int(loglikePartitions[0])].StateHistoryDepth,
	)
	nilWeights[0] = 1.0
	p.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	p.catDist = distuv.NewCategorical(nilWeights, p.Src)
}

func (p *PosteriorImportanceResampleIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logDiscount := math.Log(params.GetIndex("past_discounting_factor", 0))
	stateHistoryDepth :=
		stateHistories[int(params.GetIndex("loglike_partitions", 0))].StateHistoryDepth
	logLikes := make([]float64, 0)
	indices := make([][]int, 0)
	for i := 0; i < stateHistoryDepth; i++ {
		for j, loglikePartition := range params.Get("loglike_partitions") {
			var valueIndex int
			if v, ok := params.GetOk("loglike_indices"); ok {
				valueIndex = int(v[j])
			} else {
				valueIndex = 0
			}
			logLikes = append(
				logLikes,
				stateHistories[int(loglikePartition)].Values.At(0, valueIndex)+
					(logDiscount*float64(i)),
			)
			indices = append(indices, []int{i, j})
		}
	}
	logNorm := floats.LogSumExp(logLikes)
	for i, logLike := range logLikes {
		p.catDist.Reweight(i, math.Exp(logLike-logNorm))
	}
	indexPair := indices[int(p.catDist.Rand())]
	paramPartition := params.GetIndex("param_partitions", indexPair[1])
	sampleCentre := stateHistories[int(paramPartition)].Values.RawRowView(indexPair[0])
	normDist, ok := distmv.NewNormal(
		sampleCentre,
		mat.NewSymDense(len(sampleCentre), params.Get("sample_covariance")),
		p.Src,
	)
	if !ok {
		panic("covariance matrix is not positive-definite")
	}
	return normDist.Rand(nil)
}
