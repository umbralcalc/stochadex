package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
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
				Plotting:      &DataPlotting{IsTime: true},
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
	// GetTimeIndexFromStorage is how pkg/macros seeds a windowed simulation:
	// NewLikelihoodComparisonPartition calls it at index 0 to derive the
	// InitStateValues of each replayed data partition. Since the split that
	// is a cross-package contract, and it was previously untested.
	t.Run(
		"test that a single time index can be read from storage",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{1, 4, 7},
				{2, 5, 8},
				{3, 6, 9},
			})
			storage.SetTimes([]float64{1234, 1235, 1236})

			// A value reference yields that time index's whole row, which is
			// exactly what an InitStateValues slice needs to be.
			valueRef := &DataRef{PartitionName: "test"}
			if got := valueRef.GetTimeIndexFromStorage(storage, 1); !floats.Equal(
				got, []float64{2, 5, 8}) {
				t.Errorf("value ref at index 1 = %v, want [2 5 8]", got)
			}
			// A time reference yields a one-element slice of the time itself.
			timeRef := &DataRef{
				PartitionName: "test",
				Plotting:      &DataPlotting{IsTime: true},
			}
			if got := timeRef.GetTimeIndexFromStorage(storage, 2); !floats.Equal(
				got, []float64{1236}) {
				t.Errorf("time ref at index 2 = %v, want [1236]", got)
			}
		},
	)
	t.Run(
		"test that a configured time range clips series and rejects outside indices",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{1, 4, 7},
				{2, 5, 8},
				{3, 6, 9},
			})
			storage.SetTimes([]float64{1234, 1235, 1236})
			inRange := &DataPlotting{TimeRange: &IndexRange{Lower: 1, Upper: 3}}

			// Every series is clipped to the [Lower, Upper) window.
			valueRef := &DataRef{PartitionName: "test", Plotting: inRange}
			got := valueRef.GetFromStorage(storage)
			want := [][]float64{{2, 3}, {5, 6}, {8, 9}}
			if len(got) != len(want) {
				t.Fatalf("clipped series count = %d, want %d", len(got), len(want))
			}
			for i := range want {
				if !floats.Equal(got[i], want[i]) {
					t.Errorf("clipped series %d = %v, want %v", i, got[i], want[i])
				}
			}
			// The time axis is clipped by the same window.
			timeRef := &DataRef{
				PartitionName: "test",
				Plotting: &DataPlotting{
					IsTime:    true,
					TimeRange: &IndexRange{Lower: 1, Upper: 3},
				},
			}
			if gotTimes := timeRef.GetFromStorage(storage); len(gotTimes) != 1 ||
				!floats.Equal(gotTimes[0], []float64{1235, 1236}) {
				t.Errorf("clipped times = %v, want [[1235 1236]]", gotTimes)
			}
			// An index below Lower is outside the window and must not be
			// served silently — a caller seeding a window from it would get
			// data the range says it excluded.
			func() {
				defer func() {
					if recover() == nil {
						t.Error("reading index 0 outside range [1,3) did not panic")
					}
				}()
				ref := &DataRef{PartitionName: "test", Plotting: inRange}
				ref.GetTimeIndexFromStorage(storage, 0)
			}()
			// An index inside the window is served normally.
			ref := &DataRef{PartitionName: "test", Plotting: inRange}
			if gotRow := ref.GetTimeIndexFromStorage(storage, 1); !floats.Equal(
				gotRow, []float64{2, 5, 8}) {
				t.Errorf("in-range index 1 = %v, want [2 5 8]", gotRow)
			}
		},
	)
}
