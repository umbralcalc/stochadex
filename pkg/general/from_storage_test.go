package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

func TestFromStorage(t *testing.T) {
	t.Run(
		"test that the from storage iteration works",
		func(t *testing.T) {
			iteration := &FromStorageIteration{
				Data: [][]float64{{1.0, 2.0}, {3.0, 4.0}, {5.0, 6.0}, {7.0, 8.0}},
			}
			params := simulator.NewParams(make(map[string][]float64))
			out := iteration.Iterate(
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
			if !(out[0] == 3.0 && out[1] == 4.0) {
				t.Errorf("outputs were not as expected: %f, %f", out[0], out[1])
			}
		},
	)
	t.Run(
		"test that the from storage timestep function works",
		func(t *testing.T) {
			timestepFunction := &FromStorageTimestepFunction{
				Data: []float64{0.0, 1.0, 2.0},
			}
			out := timestepFunction.NextIncrement(
				&simulator.CumulativeTimestepsHistory{
					NextIncrement:     1.0,
					Values:            mat.NewVecDense(2, []float64{1.0, 0.0}),
					CurrentStepNumber: 2,
					StateHistoryDepth: 2,
				},
			)
			if !(out == 1.0) {
				t.Errorf("output was not as expected: %f", out)
			}
		},
	)
}
