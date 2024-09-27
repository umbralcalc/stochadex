package general

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
				"./memory_test_file.csv",
				[]int{1, 2, 3},
				true,
			)
			_ = iteration.Iterate(
				map[string][]float64{},
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
				"./memory_test_file.csv",
				0,
				true,
			)
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
