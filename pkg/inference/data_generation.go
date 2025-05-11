package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// DataGenerationIteration allows for any likelihood to be used as a
// data generation distribution based on a mean and covariance matrix.
type DataGenerationIteration struct {
	Likelihood       LikelihoodDistribution
	stepsPerResample int
}

func (d *DataGenerationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.Likelihood.SetSeed(partitionIndex, settings)
	s, ok := settings.Iterations[partitionIndex].Params.GetOk("steps_per_resample")
	if ok {
		d.stepsPerResample = int(s[0])
	} else {
		d.stepsPerResample = 1
	}
}

func (d *DataGenerationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	if timestepsHistory.CurrentStepNumber%d.stepsPerResample != 0 {
		values := make([]float64, stateHistory.StateWidth)
		copy(values, stateHistory.Values.RawRowView(0))
		return values
	}
	d.Likelihood.SetParams(
		params,
		partitionIndex,
		stateHistories,
		timestepsHistory,
	)
	samples := d.Likelihood.GenerateNewSamples()
	corr, ok := params.GetOk("correlation_with_previous")
	if ok {
		pastSamples := mat.VecDenseCopyOf(
			stateHistory.Values.RowView(0),
		)
		pastSamples.ScaleVec(corr[0], pastSamples)
		floats.Scale(1.0-corr[0], samples)
		floats.Add(samples, pastSamples.RawVector().Data)
	}
	return samples
}
