package analysis

import (
	"fmt"
	"testing"
)

func TestCsvLoading(t *testing.T) {
	t.Run(
		"test that the loading from csv file works",
		func(t *testing.T) {
			stateTimeHistories := NewStateTimeHistoriesFromCsv(
				"./test_file.csv",
				0,
				map[string][]int{
					"partition_1": {2, 1},
					"partition_2": {3},
				},
				true,
			)
			value := stateTimeHistories.StateHistories["partition_1"].Values.At(0, 0)
			if value != -6.073382023924816 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = stateTimeHistories.StateHistories["partition_1"].Values.At(0, 1)
			if value != 0.11058618396321374 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = stateTimeHistories.TimestepsHistory.Values.AtVec(0)
			if value != 99.0 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = stateTimeHistories.TimestepsHistory.Values.AtVec(99)
			if value != 0.0 {
				t.Error("csv parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
		},
	)
}
