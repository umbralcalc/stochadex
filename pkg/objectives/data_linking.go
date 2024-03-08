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

// ObjectiveIteration allows for any data linking log-likelihood to be used as a comparison
// distribution between data values and a stream of means and covariance matrices.
type ObjectiveIteration struct {
	DataLinking         DataLinkingLogLikelihood
	dataValuesPartition int
	meanPartition       int
	covMatPartition     int
	covMatOk            bool
}

func (o *ObjectiveIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	o.dataValuesPartition = int(
		settings.OtherParams[partitionIndex].IntParams["data_values_partition"][0],
	)
	o.meanPartition = int(settings.OtherParams[partitionIndex].IntParams["mean_partition"][0])
	c, covMatOk := settings.OtherParams[partitionIndex].IntParams["cov_mat_partition"]
	o.covMatOk = covMatOk
	if o.covMatOk {
		o.covMatPartition = int(c[0])
	}
	o.DataLinking.Configure(partitionIndex, settings)
}

func (o *ObjectiveIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber <
		stateHistories[o.dataValuesPartition].StateHistoryDepth {
		return []float64{0.0}
	}
	dims := stateHistories[o.meanPartition].StateWidth
	var covMat *mat.SymDense
	if o.covMatOk {
		covMat = mat.NewSymDense(
			dims,
			stateHistories[o.covMatPartition].Values.RawRowView(0),
		)
	}
	return []float64{o.DataLinking.Evaluate(
		mat.NewVecDense(
			dims,
			stateHistories[o.meanPartition].Values.RawRowView(0),
		),
		covMat,
		stateHistories[o.dataValuesPartition].Values.RawRowView(0),
	)}
}
