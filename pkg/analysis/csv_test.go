package analysis

import (
	"fmt"
	"testing"
)

func TestCsvLoading(t *testing.T) {
	t.Run(
		"test that the loading from csv file works",
		func(t *testing.T) {
			storage, _ := NewStateTimeStorageFromCsv(
				"./test_file.csv",
				0,
				map[string][]int{
					"partition_1": {2, 1},
					"partition_2": {3},
				},
				true,
			)
			value := storage.GetValues("partition_1")[0][0]
			if value != -3.26631460993812 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = storage.GetValues("partition_1")[0][1]
			if value != 0.6080753792907385 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = storage.GetTimes()[0]
			if value != 0.0 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = storage.GetTimes()[99]
			if value != 99.0 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
		},
	)
}
