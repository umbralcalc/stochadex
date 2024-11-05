package simulator

import (
	"sync"
	"testing"
)

func TestStateTimeStorage(t *testing.T) {
	t.Run(
		"test the state time storage struct works as intended",
		func(t *testing.T) {
			storage := NewStateTimeStorage()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				inc := 0.0
				for i := 0; i < 100; i++ {
					values := make([]float64, 0)
					for j := 0; j < 2; j++ {
						values = append(values, inc)
						inc += 1.0
					}
					storage.Append("test_1", float64(i), values)
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				inc := 0.0
				for i := 0; i < 100; i++ {
					values := make([]float64, 0)
					for j := 0; j < 3; j++ {
						values = append(values, inc)
						inc += 1.0
					}
					storage.Append("test_2", float64(i), values)
				}
			}()
			wg.Wait()
			inc := 0.0
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
