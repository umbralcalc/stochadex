package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// MeanFromPartition retrieves the mean from an indexed partition value.
func MeanFromPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.VecDense {
	mean := make([]float64, stateHistories[partitionIndex].StateWidth)
	copy(mean, stateHistories[int(
		params.Get("mean_partition")[0])].Values.RawRowView(0))
	return mat.NewVecDense(len(mean), mean)
}

// MeanFromParams retrieves the mean from params.
func MeanFromParams(params *simulator.Params) *mat.VecDense {
	m := params.Get("mean")
	mean := make([]float64, len(m))
	copy(mean, m)
	return mat.NewVecDense(len(mean), mean)
}

// MeanFromParamsOrPartition retrieves the mean from params or
// indexed partition value, depending on which is set.
func MeanFromParamsOrPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.VecDense {
	if _, ok := params.GetOk("mean"); ok {
		return MeanFromParams(params)
	} else if _, ok := params.GetOk("mean_partition"); ok {
		return MeanFromPartition(
			params,
			partitionIndex,
			stateHistories,
		)
	} else {
		panic("inference.MeanFromParamsOrPartition: neither" +
			" 'mean' or 'mean_partition' have been set")
	}
}

// VarianceFromParams retrieves the variance from params.
func VarianceFromParams(params *simulator.Params) *mat.VecDense {
	v := params.Get("variance")
	variance := make([]float64, len(v))
	copy(variance, v)
	return mat.NewVecDense(len(variance), variance)
}

// CovarianceMatrixFromParams retrieves the covariance matrix from params.
func CovarianceMatrixFromParams(params *simulator.Params) *mat.SymDense {
	var covMat *mat.SymDense
	if cVals, ok := params.GetOk("covariance_matrix"); ok {
		covMat = mat.NewSymDense(int(math.Sqrt(float64(len(cVals)))), cVals)
	} else if varVals, ok := params.GetOk("variance"); ok {
		dims := len(varVals)
		cVals = make([]float64, 0)
		for i := range dims {
			for j := range dims {
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
	return covMat
}
