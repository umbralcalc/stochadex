package objectives

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// DataLinkingLogLikelihood is the interface that must be implemented in
// order to create a likelihood that connects derived statistics from the
// probabilistic reweighting to observed actual data values.
type DataLinkingLogLikelihood interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	Evaluate(mean *mat.VecDense, covariance mat.Symmetric, data []float64) float64
	GenerateNewSamples(mean *mat.VecDense, covariance mat.Symmetric) []float64
}

// LastObjectiveValueIteration allows for any data linking log-likelihood to be used
// as a comparison distribution between data values and a stream of means and
// covariance matrices.
type LastObjectiveValueIteration struct {
	DataLinking         DataLinkingLogLikelihood
	dataValuesPartition int
	meanPartition       int
	covMatPartition     int
	covMatOk            bool
}

func (l *LastObjectiveValueIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	l.dataValuesPartition = int(
		settings.OtherParams[partitionIndex].IntParams["data_values_partition"][0],
	)
	l.meanPartition = int(settings.OtherParams[partitionIndex].IntParams["mean_partition"][0])
	c, covMatOk := settings.OtherParams[partitionIndex].IntParams["cov_mat_partition"]
	l.covMatOk = covMatOk
	if l.covMatOk {
		l.covMatPartition = int(c[0])
	}
	l.DataLinking.Configure(partitionIndex, settings)
}

func (l *LastObjectiveValueIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber <
		stateHistories[l.dataValuesPartition].StateHistoryDepth {
		return []float64{0.0}
	}
	dims := stateHistories[l.meanPartition].StateWidth
	var covMat *mat.SymDense
	if l.covMatOk {
		covMat = mat.NewSymDense(
			dims,
			stateHistories[l.covMatPartition].Values.RawRowView(0),
		)
	}
	return []float64{l.DataLinking.Evaluate(
		mat.NewVecDense(
			dims,
			stateHistories[l.meanPartition].Values.RawRowView(0),
		),
		covMat,
		stateHistories[l.dataValuesPartition].Values.RawRowView(0),
	)}
}
