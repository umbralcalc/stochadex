package analysis

import (
	"fmt"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestAggregation(t *testing.T) {
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
			storage = AddPartitionToStateTimeStorage(
				storage,
				aggPartition,
				map[string]int{
					"test":             2,
					"test_group":       2,
					"test_grouped_agg": 1,
				},
			)
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
	t.Run(
		"test that the mean vector works",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{1, 4, 7},
				{2, 5, 8},
				{3, 6, 9},
			})
			storage.SetTimes([]float64{1234, 1235, 1236})
			meanPartition := NewVectorMeanPartition(
				AppliedAggregation{
					Name: "test_mean",
					Data: DataRef{
						PartitionName: "test",
					},
					Kernel: &kernels.ConstantIntegrationKernel{},
				},
				storage,
			)
			storage = AddPartitionToStateTimeStorage(
				storage,
				meanPartition,
				map[string]int{
					"test":      2,
					"test_mean": 1,
				},
			)
			meanValues := storage.GetValues("test_mean")
			if !floats.Equal(meanValues[0], []float64{2, 5, 8}) {
				t.Error("data mean failed. values were: " +
					fmt.Sprint(meanValues[0]))
			}
			if !floats.Equal(meanValues[1], []float64{2, 5, 8}) {
				t.Error("data mean failed. values were: " +
					fmt.Sprint(meanValues[1]))
			}
		},
	)
	// t.Run(
	// 	"test that the variance vector works",
	// 	func(t *testing.T) {
	// 		storage := simulator.NewStateTimeStorage()
	// 		storage.SetValues("test", [][]float64{
	// 			{1, 4, 7},
	// 			{2, 5, 8},
	// 			{3, 6, 9},
	// 		})
	// 		storage.SetTimes([]float64{1234, 1235, 1236})
	// 		meanPartition := NewVectorMeanPartition(
	// 			AppliedAggregation{
	// 				Name: "test_mean",
	// 				Data: DataRef{
	// 					PartitionName: "test",
	// 				},
	// 				Kernel: &kernels.ConstantIntegrationKernel{},
	// 			},
	// 			storage,
	// 		)
	// 		storage = AddPartitionToStateTimeStorage(
	// 			storage,
	// 			meanPartition,
	// 			map[string]int{
	// 				"test":      2,
	// 				"test_mean": 1,
	// 			},
	// 		)
	// 		variancePartition := NewVectorVariancePartition(
	// 			DataRef{
	// 				PartitionName: "test_mean",
	// 			},
	// 			AppliedAggregation{
	// 				Name: "test_variance",
	// 				Data: DataRef{
	// 					PartitionName: "test",
	// 				},
	// 				Kernel: &kernels.ConstantIntegrationKernel{},
	// 			},
	// 			storage,
	// 		)
	// 		storage = AddPartitionToStateTimeStorage(
	// 			storage,
	// 			variancePartition,
	// 			map[string]int{
	// 				"test_mean":     1,
	// 				"test_variance": 2,
	// 			},
	// 		)
	// 		varianceValues := storage.GetValues("test_variance")
	// 		if !floats.Equal(varianceValues[0], []float64{2, 5, 8}) {
	// 			t.Error("data variance failed. values were: " +
	// 				fmt.Sprint(varianceValues[0]))
	// 		}
	// 		if !floats.Equal(varianceValues[1], []float64{2, 5, 8}) {
	// 			t.Error("data variance failed. values were: " +
	// 				fmt.Sprint(varianceValues[1]))
	// 		}
	// 	},
	// )
}
