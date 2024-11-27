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
	ToData    DataRef
	GroupBy   []DataRef
	Default   float64
	Precision int
}

// GroupedStateTimeStorage
type GroupedStateTimeStorage struct {
	storage  *simulator.StateTimeStorage
	defaults []float64
}

// GetStatePartitions
func (g *GroupedStateTimeStorage) GetStatePartitions() []string {
	return []string{}
}

// GetStateValueIndices
func (g *GroupedStateTimeStorage) GetStateValueIndices() []float64 {
	return []float64{}
}

// GetGroupingPartitions
func (g *GroupedStateTimeStorage) GetGroupingPartitions(tupIndex int) []string {
	return []string{}
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
