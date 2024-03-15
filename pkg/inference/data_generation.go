package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// DataGenerationIteration allows for any data linking log-likelihood to be used
// as a data generation distribution based on a mean and covariance matrix.
type DataGenerationIteration struct {
	DataLinking DataLinkingLogLikelihood
}

func (d *DataGenerationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.DataLinking.Configure(partitionIndex, settings)
}

func (d *DataGenerationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	dims := len(params.FloatParams["mean"])
	var covMat *mat.SymDense
	cVals, ok := params.FloatParams["covariance_matrix"]
	if ok {
		covMat = mat.NewSymDense(dims, cVals)
	}
	return d.DataLinking.GenerateNewSamples(
		mat.NewVecDense(
			stateHistories[partitionIndex].StateWidth,
			params.FloatParams["mean"],
		),
		covMat,
	)
}
