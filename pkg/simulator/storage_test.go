package simulator

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gonum.org/v1/gonum/floats"
)

func TestStateTimeStorageAppendByIndexRaceStress(t *testing.T) {
	const (
		partitions = 16
		steps      = 50
		stateWidth = 4
	)
	storage := NewStateTimeStorage()
	names := make([]string, partitions)
	for i := range partitions {
		names[i] = "part_" + strconv.Itoa(i)
	}
	storage.PreRegisterPartitions(names)

	var wg sync.WaitGroup
	for p := range partitions {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for step := range steps {
				row := make([]float64, stateWidth)
				for j := range stateWidth {
					row[j] = float64(step*stateWidth + j)
				}
				storage.AppendByIndex(index, float64(step), row)
			}
		}(p)
	}
	wg.Wait()

	for p := range partitions {
		rows := storage.GetValues(names[p])
		if len(rows) != steps {
			t.Fatalf("partition %s: want %d rows, got %d", names[p], steps, len(rows))
		}
	}
}

func TestStateTimeStorage(t *testing.T) {
	t.Run(
		"test the state time storage struct works as intended",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			inc := 0.0
			for i := range 100 {
				values := make([]float64, 0)
				for range 2 {
					values = append(values, inc)
					inc += 1.0
				}
				storage.Append("test_1", float64(i), values)
			}
			inc = 0.0
			for i := range 100 {
				values := make([]float64, 0)
				for range 3 {
					values = append(values, inc)
					inc += 1.0
				}
				storage.Append("test_2", float64(i), values)
			}
			inc = 0.0
			for _, values := range storage.GetValues("test_1") {
				for _, value := range values {
					if value != inc {
						t.Errorf("expected values didn't match: %f != %f", value, inc)
					}
					inc += 1.0
				}
			}
			inc = 0.0
			for _, values := range storage.GetValues("test_2") {
				for _, value := range values {
					if value != inc {
						t.Errorf("expected values didn't match: %f != %f", value, inc)
					}
					inc += 1.0
				}
			}
		},
	)
	t.Run(
		"GetValues returns a deep copy that cannot corrupt the store",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			storage.Append("test", 0.0, []float64{1.0, 2.0})

			snapshot := storage.GetValues("test")
			snapshot[0][0] = -99.0 // mutate the returned copy

			if again := storage.GetValues("test"); again[0][0] != 1.0 {
				t.Errorf(
					"mutating a GetValues result leaked into the store: got %f, want 1.0",
					again[0][0],
				)
			}
		},
	)
	t.Run(
		"GetValues panics with the available choices for an unknown name",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			storage.Append("known", 0.0, []float64{1.0})

			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected a panic for an unknown name")
				}
				got := fmt.Sprint(r)
				if !strings.Contains(got, "not found in storage") ||
					!strings.Contains(got, "known") {
					t.Errorf("unhelpful panic message: %q", got)
				}
			}()
			storage.GetValues("missing")
		},
	)
	t.Run(
		"Append records each unique timestamp once",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			// Two partitions written at the same timestamps must not duplicate
			// the shared time axis.
			storage.Append("a", 0.0, []float64{0.0})
			storage.Append("b", 0.0, []float64{0.0})
			storage.Append("a", 1.0, []float64{1.0})
			storage.Append("b", 1.0, []float64{1.0})

			times := storage.GetTimes()
			want := []float64{0.0, 1.0}
			if !floats.Equal(times, want) {
				t.Errorf("time axis: got %v, want %v", times, want)
			}
		},
	)
	t.Run(
		"SetValues and SetTimes round-trip, and GetTimes returns a copy",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			storage.SetValues("test", [][]float64{{1.0}, {2.0}, {3.0}})
			storage.SetTimes([]float64{0.0, 0.5, 1.0})

			if got := storage.GetValues("test"); len(got) != 3 || got[2][0] != 3.0 {
				t.Errorf("SetValues/GetValues round-trip failed: %v", got)
			}

			// Mutating the returned time axis must not corrupt the store.
			times := storage.GetTimes()
			times[0] = -1.0
			if again := storage.GetTimes(); again[0] != 0.0 {
				t.Errorf("GetTimes did not return a copy: got %f, want 0.0", again[0])
			}
		},
	)
	t.Run(
		"IndexOf reports registration without creating an index",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			if _, ok := storage.IndexOf("nope"); ok {
				t.Error("IndexOf reported an unregistered name as present")
			}
			storage.PreRegisterPartitions([]string{"a", "b"})
			if index, ok := storage.IndexOf("b"); !ok || index != 1 {
				t.Errorf("IndexOf(b): got (%d, %v), want (1, true)", index, ok)
			}
		},
	)
}
