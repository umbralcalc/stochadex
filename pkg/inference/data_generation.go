package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// DataGenerationIteration allows for any data-linking likelihood to be used
// as a data generation distribution based on a mean and covariance matrix.
type DataGenerationIteration struct {
	Likelihood       LikelihoodDistribution
	stepsPerResample int
}

func (d *DataGenerationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.Likelihood.Configure(partitionIndex, settings)
	s, ok := settings.OtherParams[partitionIndex].IntParams["steps_per_resample"]
	if ok {
		d.stepsPerResample = int(s[0])
	} else {
		d.stepsPerResample = 1
	}
}

func (d *DataGenerationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber%d.stepsPerResample != 0 {
		return stateHistories[partitionIndex].Values.RawRowView(0)
	}
	dims := len(params.FloatParams["mean"])
	var covMat *mat.SymDense
	cVals, ok := params.FloatParams["covariance_matrix"]
	if ok {
		covMat = mat.NewSymDense(dims, cVals)
	}
	samples := d.Likelihood.GenerateNewSamples(
		mat.NewVecDense(
			stateHistories[partitionIndex].StateWidth,
			params.FloatParams["mean"],
		),
		covMat,
	)
	corr, ok := params.FloatParams["correlation_with_previous"]
	if ok {
		pastSamples := mat.VecDenseCopyOf(
			stateHistories[partitionIndex].Values.RowView(0),
		)
		pastSamples.ScaleVec(corr[0], pastSamples)
		floats.Scale(1.0-corr[0], samples)
		floats.Add(samples, pastSamples.RawVector().Data)
	}
	return samples
}
