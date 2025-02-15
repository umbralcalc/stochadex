package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// LikelihoodDistributionWithGradient is the interface that must be implemented in
// order to create a likelihood that connects derived statistics from the
// probabilistic reweighting to observed actual data values and also provides
// a gradient of the likelihood to use in learning algorithms.
type LikelihoodDistributionWithGradient interface {
	LikelihoodDistribution
	EvaluateLogLikeGradient(
		mean *mat.VecDense,
		covariance mat.Symmetric,
		meanGrad []*mat.VecDense,
		covGrad []mat.Symmetric,
		data []float64,
	) []float64
}

// DataComparisonGradientIteration allows for any data linking log-likelihood with
// the ability to compute gradients to be used to calculate the gradient of a comparison
// distribution using data values, a mean vector, a covariance matrix and their gradients.
type DataComparisonGradientIteration struct {
	Likelihood  LikelihoodDistributionWithGradient
	burnInSteps int
	cumulative  bool
}

func (d *DataComparisonGradientIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	d.cumulative = false
	c, ok := settings.Iterations[partitionIndex].Params.GetOk("cumulative")
	if ok {
		d.cumulative = c[0] == 1
	}
	d.burnInSteps = int(
		settings.Iterations[partitionIndex].Params.GetIndex("burn_in_steps", 0))
	d.Likelihood.Configure(partitionIndex, settings)
}

func (d *DataComparisonGradientIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	if timestepsHistory.CurrentStepNumber < d.burnInSteps {
		return []float64{stateHistories[partitionIndex].Values.At(0, 0)}
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
	dimsSq := dims * dims
	gradDims := stateHistories[partitionIndex].StateWidth
	meanGrad := make([]*mat.VecDense, 0)
	meanGradData := params.Get("mean_gradient")
	cGVals, okCovG := params.GetOk("covariance_matrix_gradient")
	var varGVals []float64
	if !okCovG {
		varGVals, _ = params.GetOk("variance_gradient")
	}
	covGrad := make([]mat.Symmetric, 0)
	for di := 0; di < gradDims; di++ {
		meanGrad = append(
			meanGrad,
			mat.NewVecDense(dims, meanGradData[di*dims:(di*dims)+dims]),
		)
		var covG *mat.SymDense
		if okCovG {
			covG = mat.NewSymDense(dims, cGVals[di*dimsSq:(di*dimsSq)+dimsSq])
		} else {
			cGVals = make([]float64, 0)
			for i := 0; i < dims; i++ {
				for j := 0; j < dims; j++ {
					switch i {
					case j:
						cGVals = append(cGVals, varGVals[(di*dims)+i])
					default:
						cGVals = append(cGVals, 0.0)
					}
				}
			}
			covG = mat.NewSymDense(dims, cGVals)
		}
		covGrad = append(covGrad, covG)
	}
	likeGrad := d.Likelihood.EvaluateLogLikeGradient(
		mat.NewVecDense(dims, params.Get("mean")),
		covMat,
		meanGrad,
		covGrad,
		params.Get("latest_data_values"),
	)
	if d.cumulative {
		floats.Add(likeGrad, stateHistories[partitionIndex].Values.RawRowView(0))
	}
	return likeGrad
}
