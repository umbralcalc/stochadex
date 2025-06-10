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
	meanPartHistory := stateHistories[int(params.Get("mean_partition")[0])]
	mean := meanPartHistory.CopyStateRow(0)
	return mat.NewVecDense(len(mean), mean)
}

// MeanFromParams retrieves the mean from params.
func MeanFromParams(params *simulator.Params) *mat.VecDense {
	mean := params.GetCopy("mean")
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

// VarianceFromPartition retrieves the variance from from an indexed
// partition value.
func VarianceFromPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.VecDense {
	varPartHistory := stateHistories[int(params.Get("variance_partition")[0])]
	variance := varPartHistory.CopyStateRow(0)
	return mat.NewVecDense(len(variance), variance)
}

// VarianceFromParams retrieves the variance from params.
func VarianceFromParams(params *simulator.Params) *mat.VecDense {
	variance := params.GetCopy("variance")
	return mat.NewVecDense(len(variance), variance)
}

// VarianceFromParamsOrPartition retrieves the variance from params or
// indexed partition value, depending on which is set.
func VarianceFromParamsOrPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.VecDense {
	if _, ok := params.GetOk("variance"); ok {
		return VarianceFromParams(params)
	} else if _, ok := params.GetOk("variance_partition"); ok {
		return VarianceFromPartition(
			params,
			partitionIndex,
			stateHistories,
		)
	} else {
		panic("inference.VarianceFromParamsOrPartition: neither" +
			" 'variance' or 'variance_partition' have been set")
	}
}

// CovarianceMatrixFromPartition retrieves the covariance matrix from an
// indexed partition value.
func CovarianceMatrixFromPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.SymDense {
	var covMat *mat.SymDense
	covPart := params.Get("covariance_matrix_partition")
	covPartHistory := stateHistories[int(covPart[0])]
	cVals := covPartHistory.CopyStateRow(0)
	covMat = mat.NewSymDense(int(math.Sqrt(float64(len(cVals)))), cVals)
	return covMat
}

// CovarianceMatrixVarianceFromPartition retrieves the covariance matrix
// from an indexed partition value for the variance.
func CovarianceMatrixVarianceFromPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.SymDense {
	var covMat *mat.SymDense
	varPart := params.Get("variance_partition")
	varPartHistory := stateHistories[int(varPart[0])]
	dims := varPartHistory.StateWidth
	cVals := make([]float64, 0)
	for i := range dims {
		for j := range dims {
			switch i {
			case j:
				cVals = append(cVals, varPartHistory.Values.At(0, i))
			default:
				cVals = append(cVals, 0.0)
			}
		}
	}
	covMat = mat.NewSymDense(dims, cVals)
	return covMat
}

// CovarianceMatrixFromParams retrieves the covariance matrix from params.
func CovarianceMatrixFromParams(params *simulator.Params) *mat.SymDense {
	var covMat *mat.SymDense
	cVals := params.Get("covariance_matrix")
	covMat = mat.NewSymDense(int(math.Sqrt(float64(len(cVals)))), cVals)
	return covMat
}

// CovarianceMatrixVarianceFromParams retrieves the covariance matrix from
// variance params.
func CovarianceMatrixVarianceFromParams(params *simulator.Params) *mat.SymDense {
	var covMat *mat.SymDense
	varVals := params.Get("variance")
	dims := len(varVals)
	cVals := make([]float64, 0)
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
	return covMat
}

// CovarianceMatrixFromParamsOrPartition retrieves the covariance matrix
// from params or indexed partition value, depending on which is set.
func CovarianceMatrixFromParamsOrPartition(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
) *mat.SymDense {
	if _, ok := params.GetOk("covariance_matrix"); ok {
		return CovarianceMatrixFromParams(params)
	} else if _, ok := params.GetOk("variance"); ok {
		return CovarianceMatrixVarianceFromParams(params)
	} else if _, ok := params.GetOk("covariance_matrix_partition"); ok {
		return CovarianceMatrixFromPartition(
			params,
			partitionIndex,
			stateHistories,
		)
	} else if _, ok := params.GetOk("variance_partition"); ok {
		return CovarianceMatrixVarianceFromPartition(
			params,
			partitionIndex,
			stateHistories,
		)
	} else {
		panic("inference.CovarianceMatrixFromParamsOrPartition: none of" +
			" 'covariance_matrix', 'variance', 'covariance_matrix_partition'," +
			" or 'variance_partition' have been set")
	}
}
