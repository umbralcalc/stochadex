package analysis

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPartitions(t *testing.T) {
	t.Run(
		"test that creating a new storage from partitions works",
		func(t *testing.T) {
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{{
					Name:              "test",
					Iteration:         &general.ConstantValuesIteration{},
					Params:            simulator.NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{1.0, 2.0, 3.0},
					StateHistoryDepth: 1,
					Seed:              0,
				}},
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
				if times[i] != float64(i) {
					t.Errorf("time not expected")
				}
			}
		},
	)
	t.Run(
		"test that adding a new partition to storage works",
		func(t *testing.T) {
			storage := NewStateTimeStorageFromPartitions(
				[]*simulator.PartitionConfig{{
					Name:              "test",
					Iteration:         &general.ConstantValuesIteration{},
					Params:            simulator.NewParams(make(map[string][]float64)),
					InitStateValues:   []float64{1.0, 2.0, 3.0},
					StateHistoryDepth: 1,
					Seed:              0,
				}},
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				&simulator.ConstantTimestepFunction{
					Stepsize: 1.0,
				},
				0.0,
			)
			storage = AddPartitionToStateTimeStorage(storage, &simulator.PartitionConfig{
				Name:              "test_2",
				Iteration:         &general.ConstantValuesIteration{},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   []float64{1.0, 2.0, 3.0},
				StateHistoryDepth: 1,
				Seed:              0,
			}, nil)
			times := storage.GetTimes()
			for i, values := range storage.GetValues("test_2") {
				if values[0] != 1.0 ||
					values[1] != 2.0 ||
					values[2] != 3.0 {
					t.Errorf("values not expected")
				}
				if times[i] != float64(i) {
					t.Errorf("time not expected")
				}
			}
		},
	)
}
