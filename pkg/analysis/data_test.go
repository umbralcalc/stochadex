package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestDataRef(t *testing.T) {
	t.Run(
		"test that the data reference functionality works",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{1, 4, 7},
				{2, 5, 8},
				{3, 6, 9},
			})
			storage.SetTimes([]float64{1234, 1235, 1236})
			dataRef := &DataRef{
				PartitionName: "test",
				IsTime:        true,
			}
			names := dataRef.GetSeriesNames(storage)
			if len(names) != 1 || names[0] != "time" {
				t.Error("data ref naming failed. values were: " + strings.Join(names, ","))
			}
			values := dataRef.GetFromStorage(storage)
			if len(values) != 1 {
				t.Error("data ref failed. values have length: " + fmt.Sprintf("%d", len(values)))
			}
			compVs := 1234.0
			for _, vs := range values[0] {
				if vs != compVs {
					t.Error("data ref failed. value was: " + fmt.Sprintf("%f", vs))
				}
				compVs += 1
			}
			dataRef = &DataRef{
				PartitionName: "test",
			}
			names = dataRef.GetSeriesNames(storage)
			if len(names) != 3 || names[0] != "test 0" || names[1] != "test 1" || names[2] != "test 2" {
				t.Error("data ref naming failed. values were: " + strings.Join(names, ","))
			}
			values = dataRef.GetFromStorage(storage)
			if len(values) != 3 {
				t.Error("data ref failed. values have length: " + fmt.Sprintf("%d", len(values)))
			}
			compVs = 1.0
			for _, vss := range values {
				for _, vs := range vss {
					if vs != compVs {
						t.Error("data ref failed. value was: " +
							fmt.Sprintf("%f", vs) +
							" and expected: " +
							fmt.Sprintf("%f", compVs))
					}
					compVs += 1
				}
			}
			dataRef = &DataRef{
				PartitionName: "test",
				ValueIndices:  []int{0, 2},
			}
			names = dataRef.GetSeriesNames(storage)
			if len(names) != 2 || names[0] != "test 0" || names[1] != "test 2" {
				t.Error("data ref naming failed. values were: " + strings.Join(names, ","))
			}
			values = dataRef.GetFromStorage(storage)
			if len(values) != 2 {
				t.Error("data ref failed. values have length: " + fmt.Sprintf("%d", len(values)))
			}
			compVs = 1.0
			for _, vss := range values {
				for _, vs := range vss {
					if vs != compVs {
						t.Error("data ref failed. value was: " +
							fmt.Sprintf("%f", vs) +
							" and expected: " +
							fmt.Sprintf("%f", compVs))
					}
					compVs += 1
				}
				compVs += 3
			}
		},
	)
}
