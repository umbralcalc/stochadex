package analysis

import (
	"bytes"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCreatingPlot(t *testing.T) {
	t.Run(
		"test that rendering a scatter plot works",
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
			yRefs := make([]DataRef, 0)
			for i := range 3 {
				yRefs = append(yRefs, DataRef{
					PartitionName: "test",
					ValueIndices:  []int{i},
				})
			}
			scatter := NewScatterPlotFromPartition(
				storage,
				DataRef{
					PartitionName: "test",
					Plotting:      &DataPlotting{IsTime: true},
				},
				yRefs,
			)
			// Test by rendering to in-memory buffer
			var buf bytes.Buffer
			err := scatter.Render(&buf)
			if err != nil {
				t.Errorf("error rendering scatter plot: %v", err)
			}
			if err != nil {
				t.Error(err)
			}
		},
	)
	t.Run(
		"test that rendering a line plot works",
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
			yRefs := make([]DataRef, 0)
			for i := range 3 {
				yRefs = append(yRefs, DataRef{
					PartitionName: "test",
					ValueIndices:  []int{i},
				})
			}
			line := NewLinePlotFromPartition(
				storage,
				DataRef{
					PartitionName: "test",
					Plotting:      &DataPlotting{IsTime: true},
				},
				yRefs,
				nil,
			)
			// Test by rendering to in-memory buffer
			var buf bytes.Buffer
			err := line.Render(&buf)
			if err != nil {
				t.Errorf("error rendering line plot: %v", err)
			}
			if err != nil {
				t.Error(err)
			}
		},
	)
}
