package streamers

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

func TestCsvFileDataStreaming(t *testing.T) {
	t.Run(
		"test that the file streamer works",
		func(t *testing.T) {
			iteration := NewMemoryIterationFromCsv(
				"test_file.csv",
				[]int{1, 2, 3},
				true,
			)
			_ = iteration.Iterate(
				&simulator.OtherParams{},
				0,
				[]*simulator.StateHistory{},
				&simulator.CumulativeTimestepsHistory{
					NextIncrement:     1.0,
					Values:            mat.NewVecDense(2, []float64{1.0, 0.0}),
					CurrentStepNumber: 1,
					StateHistoryDepth: 2,
				},
			)
			timestepFunction := NewMemoryTimestepFunctionFromCsv(
				"test_file.csv",
				0,
				true,
			)
			_ = timestepFunction.SetNextIncrement(
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
