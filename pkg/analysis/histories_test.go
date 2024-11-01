package analysis

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/mat"
)

func TestStateTimeHistories(t *testing.T) {
	t.Run(
		"test that the state time histories functionality works",
		func(t *testing.T) {
			stateTimeHistories := &StateTimeHistories{
				StateHistories: map[string]*simulator.StateHistory{
					"test": {
						Values:            mat.NewDense(3, 2, []float64{5, 4, 3, 2, 1, 0}),
						NextValues:        []float64{6, 7},
						StateWidth:        2,
						StateHistoryDepth: 3,
					},
				},
				TimestepsHistory: &simulator.CumulativeTimestepsHistory{
					NextIncrement:     1,
					Values:            mat.NewVecDense(3, []float64{236, 235, 234}),
					CurrentStepNumber: 237,
					StateHistoryDepth: 3,
				},
			}
			df := stateTimeHistories.GetDataFrameFromPartition("test")
			value := df.Elem(0, 0)
			if value.Float() != 236 {
				t.Error("df creation failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = df.Elem(0, 1)
			if value.Float() != 5 {
				t.Error("df creation failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = df.Elem(0, 2)
			if value.Float() != 4 {
				t.Error("df creation failed. value was: " + fmt.Sprintf("%f", value))
			}
			df.Elem(1, 1).Set(12345678)
			stateTimeHistories.SetPartitionFromDataFrame(df, "test", true)
			df = stateTimeHistories.GetDataFrameFromPartition("test")
			value = df.Elem(1, 1)
			if value.Float() != 12345678 {
				t.Error("df setting failed. value was: " + fmt.Sprintf("%f", value))
			}
		},
	)
}
