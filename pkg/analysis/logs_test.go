package analysis

import (
	"fmt"
	"testing"
)

func TestJsonLogsLoading(t *testing.T) {
	t.Run(
		"test that the loading from json logs file works",
		func(t *testing.T) {
			stateTimeHistories, _ := NewStateTimeHistoriesFromJsonLogEntries(
				"./test_file.log",
				50,
			)
			value := stateTimeHistories.StateHistories["first_wiener_process"].Values.At(16, 0)
			if value != 1.4413432494888023 {
				t.Error("json logs parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = stateTimeHistories.StateHistories["second_wiener_process"].Values.At(37, 1)
			if value != -2.2900800045813647 {
				t.Error("json logs parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = stateTimeHistories.TimestepsHistory.Values.AtVec(9)
			if value != 41.0 {
				t.Error("json logs parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
		},
	)
}
