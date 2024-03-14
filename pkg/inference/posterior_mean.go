package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// PosteriorMeanIteration
type PosteriorMeanIteration struct {
	objectivePartition  int
	valuesPartition     int
	otherMeanPartitions []int
}

func (p *PosteriorMeanIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.otherMeanPartitions = make([]int, 0)
	for _, meanPart := range settings.OtherParams[partitionIndex].
		IntParams["other_mean_partitions"] {
		p.otherMeanPartitions = append(p.otherMeanPartitions, int(meanPart))
	}
	p.valuesPartition = int(settings.OtherParams[partitionIndex].
		IntParams["values_partition"][0])
	p.objectivePartition = int(settings.OtherParams[partitionIndex].
		IntParams["objective_partition"][0])
}

func (p *PosteriorMeanIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	lastMean := mat.VecDenseCopyOf(stateHistories[partitionIndex].Values.RowView(0))
	for _, meanPart := range p.otherMeanPartitions {
		lastMean.AddVec(lastMean, stateHistories[meanPart].Values.RowView(0))
	}
	lastMean.ScaleVec(1.0/float64(len(p.otherMeanPartitions)+1), lastMean)
	return make([]float64, 0)
}
