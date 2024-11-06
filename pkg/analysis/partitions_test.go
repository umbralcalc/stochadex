package analysis

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestStorageFromPartition(t *testing.T) {
	t.Run(
		"test that creating a new storage from partition works",
		func(t *testing.T) {
			storage := NewStateTimeStorageFromPartition(
				&simulator.PartitionConfig{
					Name:              "test",
					Iteration:         &general.ConstantValuesIteration{},
					Params:            simulator.NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{1.0, 2.0, 3.0},
					StateHistoryDepth: 1,
					Seed:              0,
				},
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				&simulator.ConstantTimestepFunction{
					Stepsize: 1.0,
				},
				0.0,
			)
			times := storage.GetTimes()
			for i, values := range storage.GetValues("test") {
				if values[0] != 1.0 ||
					values[1] != 2.0 ||
					values[2] != 3.0 {
					t.Errorf("values not expected")
				}
				if times[i] != float64(i+1) {
					t.Errorf("time not expected")
				}
			}
		},
	)
}
