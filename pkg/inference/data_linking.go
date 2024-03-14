package inference

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
	DataLinking DataLinkingLogLikelihood
	burnInSteps int
}

func (l *LastObjectiveValueIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	l.burnInSteps = int(
		settings.OtherParams[partitionIndex].IntParams["burn_in_steps"][0],
	)
	l.DataLinking.Configure(partitionIndex, settings)
}

func (l *LastObjectiveValueIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber < l.burnInSteps {
		return []float64{0.0}
	}
	dims := len(params.FloatParams["mean"])
	var covMat *mat.SymDense
	cVals, ok := params.FloatParams["covariance_matrix"]
	if ok {
		covMat = mat.NewSymDense(dims, cVals)
	}
	return []float64{l.DataLinking.Evaluate(
		mat.NewVecDense(
			dims,
			params.FloatParams["mean"],
		),
		covMat,
		params.FloatParams["latest_data_values"],
	)}
}
