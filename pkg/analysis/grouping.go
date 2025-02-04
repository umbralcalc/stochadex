package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AppliedGrouping configures a grouping transformation on data.
type AppliedGrouping struct {
	GroupBy   []DataRef
	Precision int
}

// GroupedStateTimeStorage is a representation of simulator.StateTimeStorage
// which has already had a grouping transformation applied to it.
type GroupedStateTimeStorage struct {
	Storage        *simulator.StateTimeStorage
	applied        AppliedGrouping
	acceptedGroups [][]float64
	groupLabels    []string
}

// GetGroupingPartitions returns the partition used in the data for grouping.
func (g *GroupedStateTimeStorage) GetGroupingPartition(tupIndex int) string {
	return g.applied.GroupBy[tupIndex].PartitionName
}

// GetGroupingValueIndices returns the value indices used in the data for grouping.
func (g *GroupedStateTimeStorage) GetGroupingValueIndices(tupIndex int) []float64 {
	valueIndices := make([]float64, 0)
	for _, index := range g.applied.GroupBy[tupIndex].GetValueIndices(g.Storage) {
		valueIndices = append(valueIndices, float64(index))
	}
	return valueIndices
}

// GetAcceptedValueGroups returns the unique groups that were found in the data
// which are typically used to configure group aggregation partitions.
func (g *GroupedStateTimeStorage) GetAcceptedValueGroups(tupIndex int) []float64 {
	groupAtIndex := make([]float64, 0)
	for _, group := range g.acceptedGroups {
		groupAtIndex = append(groupAtIndex, group[tupIndex])
	}
	return groupAtIndex
}

// GetAcceptedValueGroupLabels returns the unique group labels that were found in
// the data which are typically used for labelling plots.
func (g *GroupedStateTimeStorage) GetAcceptedValueGroupLabels() []string {
	return g.groupLabels
}

// GetAcceptedValueGroupsLength returns the number of accepted value groups (equivalent
// to the length of the state vector in simulation partition).
func (g *GroupedStateTimeStorage) GetAcceptedValueGroupsLength() int {
	return len(g.acceptedGroups)
}

// GetGroupTupleLength returns the length of tuple in the grouping index construction.
func (g *GroupedStateTimeStorage) GetGroupTupleLength() int {
	return len(g.applied.GroupBy)
}

// GetPrecision returns the requested float precision for grouping.
func (g *GroupedStateTimeStorage) GetPrecision() int {
	return g.applied.Precision
}

// NewGroupedStateTimeStorage creates a new GroupedStateTimeStorage given
// the provided simulator.StateTimeStorage and applied grouping.
func NewGroupedStateTimeStorage(
	applied AppliedGrouping,
	storage *simulator.StateTimeStorage,
) *GroupedStateTimeStorage {
	valuesByGroup := make([][][]float64, 0)
	for _, ref := range applied.GroupBy {
		valuesByGroup = append(valuesByGroup, ref.GetFromStorage(storage))
	}
	uniqueGroups := make(map[string]bool)
	groupLabels := make([]string, 0)
	acceptedGroups := make([][]float64, 0)
	var ok bool
	var key string
	var groupTuple []float64
	precision := applied.Precision
	for i, values := range valuesByGroup[0] {
		for j := range values {
			key = ""
			groupTuple = make([]float64, 0)
			for _, groupValue := range valuesByGroup {
				val := groupValue[i][j]
				groupTuple = append(groupTuple, val)
				key = general.AppendFloatToKey(key, val, precision)
			}
			if _, ok = uniqueGroups[key]; !ok {
				groupLabels = append(groupLabels, key)
				acceptedGroups = append(acceptedGroups, groupTuple)
				uniqueGroups[key] = true
			}
		}
	}
	return &GroupedStateTimeStorage{
		Storage:        storage,
		applied:        applied,
		acceptedGroups: acceptedGroups,
		groupLabels:    groupLabels,
	}
}
