package simulator

import (
	"testing"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

func TestHistories(t *testing.T) {
	t.Run(
		"test the state histories struct works as intended",
		func(t *testing.T) {
			history := StateHistory{
				Values:            mat.NewDense(3, 2, []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0}),
				StateWidth:        2,
				StateHistoryDepth: 3,
			}
			copyMiddleRow := history.CopyStateRow(1)
			if !floats.Equal(copyMiddleRow, []float64{3.0, 4.0}) {
				t.Error("middle row not copied correctly")
			}
			copyMiddleRow[0] = 5.0
			copyMiddleRow[1] = 10.0
			secondCopyMiddleRow := history.CopyStateRow(1)
			if !floats.Equal(secondCopyMiddleRow, []float64{3.0, 4.0}) {
				t.Error("middle row isn't a proper copy")
			}
			if !floats.Equal(history.GetNextStateRowToUpdate(), []float64{1.0, 2.0}) {
				t.Error("most recent row not copied correctly")
			}
		},
	)
}
