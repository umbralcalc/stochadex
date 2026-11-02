package analysis

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
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
	// The accessors below are the contract pkg/macros builds grouped
	// aggregation partitions from: NewGroupedAggregationPartition loops
	// `for tupIndex := range GetGroupTupleLength()` and calls the per-tuple
	// accessors to size and wire each partition. That makes it a
	// cross-package contract, so it is tested against a multi-dimensional
	// grouping — a single GroupBy ref cannot distinguish a correct tupIndex
	// from an ignored one.
	t.Run(
		"test that the grouping accessors describe a multi-dimensional grouping",
		func(t *testing.T) {
			storage := simulator.NewStateTimeStorage()
			// Rows are time indices, columns are value indices. Two group
			// refs of matching shape give four distinct (ga, gb) tuples.
			storage.SetValues("ga", [][]float64{
				{1, 1},
				{2, 2},
			})
			storage.SetValues("gb", [][]float64{
				{10, 20},
				{10, 20},
			})
			storage.SetTimes([]float64{100, 200})
			grouped := NewGroupedStateTimeStorage(
				AppliedGrouping{
					GroupBy: []DataRef{
						{PartitionName: "ga"},
						{PartitionName: "gb"},
					},
					Precision: 3,
				},
				storage,
			)

			if got := grouped.GetGroupTupleLength(); got != 2 {
				t.Errorf("group tuple length = %d, want 2", got)
			}
			if got := grouped.GetPrecision(); got != 3 {
				t.Errorf("precision = %d, want 3", got)
			}
			if got := grouped.GetAcceptedValueGroupsLength(); got != 4 {
				t.Errorf("accepted value groups length = %d, want 4", got)
			}
			for tupIndex, want := range map[int]string{0: "ga", 1: "gb"} {
				if got := grouped.GetGroupingPartition(tupIndex); got != want {
					t.Errorf("grouping partition %d = %q, want %q",
						tupIndex, got, want)
				}
			}
			// ValueIndices was left nil, so it defaults to every index in
			// the partition — two columns each.
			for tupIndex := range 2 {
				got := grouped.GetGroupingValueIndices(tupIndex)
				if !floats.Equal(got, []float64{0, 1}) {
					t.Errorf("grouping value indices %d = %v, want [0 1]",
						tupIndex, got)
				}
			}
			// The four tuples are (1,10), (2,10), (1,20), (2,20), in the
			// order the (series, time) scan discovers them.
			wantGroups := map[int][]float64{
				0: {1, 2, 1, 2},
				1: {10, 10, 20, 20},
			}
			for tupIndex, want := range wantGroups {
				got := grouped.GetAcceptedValueGroups(tupIndex)
				if !floats.Equal(got, want) {
					t.Errorf("accepted value groups %d = %v, want %v",
						tupIndex, got, want)
				}
			}
			// The invariant macros actually relies on: every tuple position
			// yields one value per accepted group, so a partition sized by
			// GetAcceptedValueGroupsLength lines up with the group values
			// used to configure it.
			for tupIndex := range grouped.GetGroupTupleLength() {
				if got := len(grouped.GetAcceptedValueGroups(tupIndex)); got !=
					grouped.GetAcceptedValueGroupsLength() {
					t.Errorf("tuple %d has %d group values, but there are %d groups",
						tupIndex, got, grouped.GetAcceptedValueGroupsLength())
				}
			}
		},
	)
}

func TestDeclaredGroupedStateTimeStorage(t *testing.T) {
	storage := simulator.NewStateTimeStorage()
	storage.SetValues("a", [][]float64{{1, 2}, {3, 4}})
	storage.SetValues("b", [][]float64{{5, 6}, {7, 8}})
	storage.SetTimes([]float64{0, 1})
	applied := AppliedGrouping{
		GroupBy:   []DataRef{{PartitionName: "a"}, {PartitionName: "b", ValueIndices: []int{1}}},
		Precision: 1,
	}
	// Declared groups need not occur in the data, and keep the order given.
	grouped := NewDeclaredGroupedStateTimeStorage(applied, storage, [][]float64{{9, 0.25}, {1, 5}})
	if n := grouped.GetAcceptedValueGroupsLength(); n != 2 {
		t.Fatalf("%d groups, want the 2 declared", n)
	}
	if got := grouped.GetAcceptedValueGroups(0); !floats.Equal(got, []float64{9, 1}) {
		t.Errorf("first tuple values %v, want [9 1]", got)
	}
	if got := grouped.GetAcceptedValueGroups(1); !floats.Equal(got, []float64{0.25, 5}) {
		t.Errorf("second tuple values %v, want [0.25 5]", got)
	}
	if got := strings.Join(grouped.GetAcceptedValueGroupLabels(), " "); got != "9.0,0.2, 1.0,5.0," {
		t.Errorf("labels %q, want the declared tuples formatted at precision 1", got)
	}
	if grouped.GetGroupTupleLength() != 2 || grouped.GetPrecision() != 1 ||
		grouped.GetGroupingPartition(1) != "b" || !floats.Equal(grouped.GetGroupingValueIndices(1), []float64{1}) {
		t.Error("the grouping itself should be the applied one")
	}
}
