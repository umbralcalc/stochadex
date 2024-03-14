package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

// DataGenerationIteration allows for any data linking log-likelihood to be used
// as a data generation distribution based on input mean and a covariance matrix.
type DataGenerationIteration struct {
	DataLinking DataLinkingLogLikelihood
	mean        *mat.VecDense
	covMatrix   *mat.SymDense
}

func (d *DataGenerationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	stateWidth := settings.StateWidths[partitionIndex]
	d.mean = mat.NewVecDense(
		stateWidth,
		settings.OtherParams[partitionIndex].FloatParams["mean"],
	)
	d.covMatrix = mat.NewSymDense(stateWidth, nil)
	up, ok := settings.OtherParams[partitionIndex].
		FloatParams["upper_triangle_cholesky_of_cov_matrix"]
	if ok {
		row := 0
		col := 0
		upperTri := mat.NewTriDense(stateWidth, mat.Upper, nil)
		for _, param := range up {
			// nonzero values along the diagonal are needed as a constraint
			if col == row && param == 0.0 {
				param = 1e-4
			}
			upperTri.SetTri(row, col, param)
			col += 1
			if col == stateWidth {
				row += 1
				col = row
			}
		}
		var choleskyDecomp mat.Cholesky
		choleskyDecomp.SetFromU(upperTri)
		choleskyDecomp.ToSym(d.covMatrix)
	}
	d.DataLinking.Configure(partitionIndex, settings)
}

func (d *DataGenerationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return d.DataLinking.GenerateNewSamples(d.mean, d.covMatrix)
}
