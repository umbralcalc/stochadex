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
				Values: mat.NewDense(
					4,
					2,
					[]float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0},
				),
				NextValues:        []float64{1.0, 2.0},
				StateWidth:        2,
				StateHistoryDepth: 4,
			}}
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
			if !(out[0] == 5.0 && out[1] == 6.0) {
				t.Errorf("outputs were not as expected: %f, %f", out[0], out[1])
			}
			iteration.UpdateMemory(
				&params,
				&StateMemoryUpdate{
					Name: "test",
					StateHistory: &simulator.StateHistory{

						Values: mat.NewDense(
							4,
							2,
							[]float64{9.0, 10.0, 11.0, 12.0, 13.0, 14.0, 15.0, 16.0},
						),
						NextValues:        []float64{1.0, 2.0},
						StateWidth:        2,
						StateHistoryDepth: 4,
					},
					TimestepsHistory: &simulator.CumulativeTimestepsHistory{
						NextIncrement:     1.0,
						Values:            mat.NewVecDense(2, []float64{1.0, 0.0}),
						CurrentStepNumber: 1,
						StateHistoryDepth: 2,
					},
				},
			)
			out = iteration.Iterate(
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
			if !(out[0] == 13.0 && out[1] == 14.0) {
				t.Errorf("outputs were not as expected: %f, %f", out[0], out[1])
			}
		},
	)
	t.Run(
		"test that the from history timestep function works",
		func(t *testing.T) {
			timestepFunction := &FromHistoryTimestepFunction{
				Data: &simulator.CumulativeTimestepsHistory{
					Values:            mat.NewVecDense(3, []float64{2.0, 1.0, 0.0}),
					StateHistoryDepth: 3,
				},
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
