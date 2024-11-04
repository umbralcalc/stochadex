package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

func TestFromHistory(t *testing.T) {
	t.Run(
		"test that the from history iteration works",
		func(t *testing.T) {
			iteration := &FromHistoryIteration{Data: &simulator.StateHistory{
				Values:            mat.NewDense(3, 2, []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0}),
				NextValues:        []float64{1.0, 2.0, 3.0},
				StateWidth:        3,
				StateHistoryDepth: 2,
			}}
			params := simulator.NewParams(make(map[string][]float64))
			_ = iteration.Iterate(
				&params,
				0,
				[]*simulator.StateHistory{},
				&simulator.CumulativeTimestepsHistory{
					NextIncrement:     1.0,
					Values:            mat.NewVecDense(2, []float64{1.0, 0.0}),
					CurrentStepNumber: 1,
					StateHistoryDepth: 2,
				},
			)
		},
	)
	t.Run(
		"test that the from history timestep function works",
		func(t *testing.T) {
			timestepFunction := &FromHistoryTimestepFunction{
				Data: &simulator.CumulativeTimestepsHistory{
					NextIncrement:     1.0,
					Values:            mat.NewVecDense(2, []float64{1.0, 0.0}),
					CurrentStepNumber: 1,
					StateHistoryDepth: 2,
				},
			}
			_ = timestepFunction.NextIncrement(
				&simulator.CumulativeTimestepsHistory{
					NextIncrement:     1.0,
					Values:            mat.NewVecDense(2, []float64{1.0, 0.0}),
					CurrentStepNumber: 1,
					StateHistoryDepth: 2,
				},
			)
		},
	)
}
