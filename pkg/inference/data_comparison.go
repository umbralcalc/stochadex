package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// LikelihoodDistribution is the interface that must be implemented in
// order to create a likelihood that connects derived statistics from the
// probabilistic reweighting to observed actual data values.
type LikelihoodDistribution interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	EvaluateLogLike(mean *mat.VecDense, covariance mat.Symmetric, data []float64) float64
	GenerateNewSamples(mean *mat.VecDense, covariance mat.Symmetric) []float64
}

// DataComparisonIteration allows for any data linking log-likelihood to be used
// as a comparison distribution between data values, a mean vector and covariance
// matrix.
type DataComparisonIteration struct {
	Likelihood  LikelihoodDistribution
	burnInSteps int
}

func (d *DataComparisonIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.burnInSteps = int(
		settings.OtherParams[partitionIndex].IntParams["burn_in_steps"][0],
	)
	d.Likelihood.Configure(partitionIndex, settings)
}

func (d *DataComparisonIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber < d.burnInSteps {
		return []float64{0.0}
	}
	dims := len(params.FloatParams["mean"])
	var covMat *mat.SymDense
	cVals, ok := params.FloatParams["covariance_matrix"]
	if ok {
		covMat = mat.NewSymDense(dims, cVals)
	}
	return []float64{d.Likelihood.EvaluateLogLike(
		mat.NewVecDense(
			dims,
			params.FloatParams["mean"],
		),
		covMat,
		params.FloatParams["latest_data_values"],
	)}
}