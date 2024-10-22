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
	s, ok := settings.Params[partitionIndex].GetOk("steps_per_resample")
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
	if timestepsHistory.CurrentStepNumber%d.stepsPerResample != 0 {
		return stateHistories[partitionIndex].Values.RawRowView(0)
	}
	dims := len(params.Get("mean"))
	var covMat *mat.SymDense
	cVals, ok := params.GetOk("covariance_matrix")
	if ok {
		covMat = mat.NewSymDense(dims, cVals)
	} else if varVals, ok := params.GetOk("variance"); ok {
		cVals = make([]float64, 0)
		for i := 0; i < dims; i++ {
			for j := 0; j < dims; j++ {
				switch i {
				case j:
					cVals = append(cVals, varVals[i])
				default:
					cVals = append(cVals, 0.0)
				}
			}
		}
		covMat = mat.NewSymDense(dims, cVals)
	}
	samples := d.Likelihood.GenerateNewSamples(
		mat.NewVecDense(
			stateHistories[partitionIndex].StateWidth,
			params.Get("mean"),
		),
		covMat,
	)
	corr, ok := params.GetOk("correlation_with_previous")
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
