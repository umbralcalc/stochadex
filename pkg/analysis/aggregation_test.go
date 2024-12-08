package analysis

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestGroupedAggregation(t *testing.T) {
	t.Run(
		"test that the grouped aggregation works",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{1, 4, 7},
				{2, 5, 8},
				{3, 6, 9},
			})
			storage.SetValues("test_group", [][]float64{
				{1, 2, 4},
				{1, 2, 4},
				{2, 3, 4},
			})
			storage.SetTimes([]float64{1234, 1235, 1236})
			groupedStorage := NewGroupedStateTimeStorage(
				AppliedGrouping{
					GroupBy: []DataRef{
						{PartitionName: "test_group"},
					},
					Default:   0.0,
					Precision: 1,
				},
				storage,
			)
			aggPartition := NewGroupedAggregationPartition(
				general.MaxAggregation,
				AppliedAggregation{
					Name: "test_grouped_agg",
					Data: DataRef{
						PartitionName: "test",
					},
					Kernel: &kernels.InstantaneousIntegrationKernel{},
				},
				groupedStorage,
			)
			storage = AddPartitionToStateTimeStorage(storage, aggPartition, nil)
			groupValues := storage.GetValues("test_grouped_agg")
			if !floats.Equal(groupValues[0], []float64{2, 5, 0, 8}) {
				t.Error("data grouped aggregation failed. values were: " +
					fmt.Sprint(groupValues[0]))
			}
			if !floats.Equal(groupValues[1], []float64{0, 3, 6, 9}) {
				t.Error("data grouped aggregation failed. values were: " +
					fmt.Sprint(groupValues[1]))
			}
		},
	)
}
