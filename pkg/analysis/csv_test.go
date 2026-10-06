package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// writeTempCsv writes contents to a CSV file in a per-test temp dir and
// returns its path.
func writeTempCsv(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing temp csv: %v", err)
	}
	return path
}

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
	t.Run(
		"test that a well-formed csv loads its times and values exactly",
		func(t *testing.T) {
			path := writeTempCsv(t, "t,x,y\n0.5,1.0,2.0\n1.5,3.0,4.0\n2.5,5.0,6.0\n")
			storage, err := NewStateTimeStorageFromCsv(
				path, 0, map[string][]int{"p": {1, 2}}, true,
			)
			if err != nil {
				t.Fatalf("unexpected error loading well-formed csv: %v", err)
			}
			if times := storage.GetTimes(); !floats.Equal(times, []float64{0.5, 1.5, 2.5}) {
				t.Errorf("times = %v, want [0.5 1.5 2.5]", times)
			}
			want := [][]float64{{1, 2}, {3, 4}, {5, 6}}
			got := storage.GetValues("p")
			if len(got) != len(want) {
				t.Fatalf("got %d rows, want %d", len(got), len(want))
			}
			for i := range want {
				if !floats.Equal(got[i], want[i]) {
					t.Errorf("row %d = %v, want %v", i, got[i], want[i])
				}
			}
		},
	)
	t.Run(
		"test that a non-numeric time value returns an error naming the row and value",
		func(t *testing.T) {
			// The bad value is on the third row of the file (after the
			// header and one good row). Before the fix this row was kept
			// with time 0, silently corrupting the time axis.
			path := writeTempCsv(t, "t,x\n0.5,1.0\nnot-a-time,2.0\n2.5,3.0\n")
			storage, err := NewStateTimeStorageFromCsv(
				path, 0, map[string][]int{"p": {1}}, true,
			)
			if err == nil {
				t.Fatalf("expected an error for non-numeric time, got storage with times %v",
					storage.GetTimes())
			}
			if storage != nil {
				t.Errorf("expected nil storage alongside the error")
			}
			for _, want := range []string{"row 3", `"not-a-time"`} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %s", err, want)
				}
			}
		},
	)
}
