package analysis

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestGroupedStateTimeStorage(t *testing.T) {
	t.Run(
		"test that the grouped state time storage works",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			storage.SetValues("test", [][]float64{
				{1, 4, 7},
				{2, 5, 8},
				{3, 6, 9},
			})
			storage.SetValues("test_group", [][]float64{
				{1.2, 2.02, 4.004},
				{1.1, 2.03, 4.0},
				{2.01, 3.1, 4.1},
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
			if len(groupedStorage.GetAcceptedValueGroupLabels()) != 6 {
				t.Error("data grouping failed. labels were: " +
					strings.Join(groupedStorage.GetAcceptedValueGroupLabels(), ""))
			}
			groupedStorage = NewGroupedStateTimeStorage(
				AppliedGrouping{
					GroupBy: []DataRef{
						{PartitionName: "test_group"},
					},
					Default:   0.0,
					Precision: 2,
				},
				storage,
			)
			if len(groupedStorage.GetAcceptedValueGroupLabels()) != 8 {
				t.Error("data grouping failed. labels were: " +
					strings.Join(groupedStorage.GetAcceptedValueGroupLabels(), " "))
			}
		},
	)
}
