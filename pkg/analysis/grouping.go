package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// KeyedGroup
type KeyedGroup struct {
	Key   string
	Group []float64
}

// AppliedGrouping
type AppliedGrouping struct {
	GroupBy   []DataRef
	Default   float64
	Precision int
}

// GroupedStateTimeStorage
type GroupedStateTimeStorage struct {
	storage  *simulator.StateTimeStorage
	defaults []float64
}

// GetGroupingPartitions
func (g *GroupedStateTimeStorage) GetGroupingPartition(tupIndex int) string {
	return ""
}

// GetGroupingValueIndices
func (g *GroupedStateTimeStorage) GetGroupingValueIndices(tupIndex int) []float64 {
	return []float64{}
}

// GetAcceptedValueGroups
func (g *GroupedStateTimeStorage) GetAcceptedValueGroups(tupIndex int) []float64 {
	return []float64{}
}

// GetGroupTupleLength
func (g *GroupedStateTimeStorage) GetGroupTupleLength() int {
	return 1
}

// GetPrecision
func (g *GroupedStateTimeStorage) GetPrecision() int {
	return 1
}

// GetDefaults
func (g *GroupedStateTimeStorage) GetDefaults() []float64 {
	if g.defaults == nil {
		// fill this with zeros as default
		return []float64{}
	}
	return g.defaults
}

// NewGroupedStateTimeStorage creates a new GroupedStateTimeStorage given
// the provided simulator.StateTimeStorage and applied grouping.
func NewGroupedStateTimeStorage(
	applied AppliedGrouping,
	storage *simulator.StateTimeStorage,
) *GroupedStateTimeStorage {
	var defaults []float64
	return &GroupedStateTimeStorage{
		storage:  storage,
		defaults: defaults,
	}
}
