package inference

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// PosteriorKernelUpdateIteration computes updates to the degrees of freedom
// and scale matrix of the posterior t-distribution kernel.
type PosteriorKernelUpdateIteration struct {
	timeRange *general.ValuesFunctionTimeDeltaRange
}

func (p *PosteriorKernelUpdateIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	if lowerUpper, ok := settings.Iterations[partitionIndex].Params.GetOk(
		"delta_time_range"); ok {
		p.timeRange = &general.ValuesFunctionTimeDeltaRange{
			LowerDelta: lowerUpper[0],
			UpperDelta: lowerUpper[1],
		}
	}
}

func (p *PosteriorKernelUpdateIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	dof := stateHistory.Values.At(0, stateHistory.StateWidth-1)
	scaleMatrixValues := make([]float64, stateHistory.StateWidth-1)
	copy(
		scaleMatrixValues,
		stateHistory.Values.RawRowView(0)[:stateHistory.StateWidth-1],
	)
	scaleMatrix := mat.NewSymDense(
		int(math.Sqrt(float64(stateHistory.StateWidth-1))),
		scaleMatrixValues,
	)
	discount := params.Get("past_discounting_factor")[0]
	latestStateValues := params.Get("latest_data_values")
	dataStateHistory := stateHistories[int(
		params.GetIndex("data_values_partition", 0))]
	latestTime := timestepsHistory.Values.AtVec(0) +
		timestepsHistory.NextIncrement
	appliedDiscount := false
	diffValues := make([]float64, len(latestStateValues))
	var timeDelta, pastTime float64
	for i := range dataStateHistory.StateHistoryDepth {
		pastTime = timestepsHistory.Values.AtVec(i)
		if p.timeRange != nil {
			timeDelta = latestTime - pastTime
			if p.timeRange.LowerDelta > timeDelta ||
				timeDelta >= p.timeRange.UpperDelta {
				continue
			}
		}
		if !appliedDiscount {
			dof *= discount
			scaleMatrix.ScaleSym(discount, scaleMatrix)
			appliedDiscount = true
		}
		dof = dof + 1.0
		scaleMatrix.SymRankOne(
			scaleMatrix,
			1.0,
			mat.NewVecDense(
				len(latestStateValues),
				floats.SubTo(
					diffValues,
					latestStateValues,
					dataStateHistory.Values.RawRowView(i),
				),
			),
		)
	}
	return append(scaleMatrix.RawSymmetric().Data, dof)
}
