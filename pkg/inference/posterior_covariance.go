package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// PosteriorCovarianceIteration
type PosteriorCovarianceIteration struct {
	objectivePartition    int
	valuesPartition       int
	otherCovMatPartitions []int
}

func (p *PosteriorCovarianceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p.otherCovMatPartitions = make([]int, 0)
	for _, covPart := range settings.OtherParams[partitionIndex].
		IntParams["other_covariance_partitions"] {
		p.otherCovMatPartitions = append(p.otherCovMatPartitions, int(covPart))
	}
	p.valuesPartition = int(settings.OtherParams[partitionIndex].
		IntParams["values_partition"][0])
	p.objectivePartition = int(settings.OtherParams[partitionIndex].
		IntParams["objective_partition"][0])
}

func (p *PosteriorCovarianceIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	lastCovValues := mat.VecDenseCopyOf(stateHistories[partitionIndex].Values.RowView(0))
	for _, covPart := range p.otherCovMatPartitions {
		lastCovValues.AddVec(lastCovValues, stateHistories[covPart].Values.RowView(0))
	}
	lastCovValues.ScaleVec(1.0/float64(len(p.otherCovMatPartitions)+1), lastCovValues)
	return make([]float64, 0)
}
