package analysis

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestDataFrameLoading(t *testing.T) {
	t.Run(
		"test that the dataframe loading functionality works",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{5, 4},
				{3, 2},
				{1, 0},
			})
			storage.SetTimes([]float64{236, 235, 234})
			df := GetDataFrameFromPartition(storage, "test")
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
			SetPartitionFromDataFrame(storage, "test", df, true)
			df = GetDataFrameFromPartition(storage, "test")
			value = df.Elem(1, 1)
			if value.Float() != 12345678 {
				t.Error("df setting failed. value was: " + fmt.Sprintf("%f", value))
			}
		},
	)
}
