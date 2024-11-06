package analysis

import (
	"fmt"
	"testing"
)

func TestJsonLogsLoading(t *testing.T) {
	t.Run(
		"test that the loading from json logs file works",
		func(t *testing.T) {
			storage, _ := NewStateTimeStorageFromJsonLogEntries("./test_file.log")
			value := storage.GetValues("first_wiener_process")[16][0]
			if value != -0.6861592904514444 {
				t.Error("json logs parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = storage.GetValues("second_wiener_process")[37][1]
			if value != 1.6770557129529018 {
				t.Error("json logs parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
			value = storage.GetTimes()[9]
			if value != 10.0 {
				t.Error("json logs parsing failed. value was: " + fmt.Sprintf("%f", value))
			}
		},
	)
}
