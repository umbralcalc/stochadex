package simulator

import (
	"strconv"
	"sync"
	"testing"
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
}
