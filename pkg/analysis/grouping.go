package analysis

import (
	"fmt"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// KeyedGroup
type KeyedGroup struct {
	Key   string
	Group []float64
}

// RoundToPrecision rounds floats to n decimal places.
func RoundToPrecision(value float64, precision int) float64 {
	format := "%." + strconv.Itoa(precision) + "f"
	roundedValue, _ := strconv.ParseFloat(fmt.Sprintf(format, value), 64)
	return roundedValue
}

// FloatTupleToKey converts a slice of floats to a string key with
// fixed precision for float values.
func FloatTupleToKey(tuple []float64, precision int) string {
	key := ""
	for _, v := range tuple {
		rounded := RoundToPrecision(v, precision)
		key += strconv.FormatFloat(rounded, 'f', precision, 64) + ","
	}
	return key
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

// GetGrouping
func (g *GroupedStateTimeStorage) GetGrouping() func(
	params *simulator.Params,
	stateHistories []*simulator.StateHistory,
) general.Groupings {
	return func(
		params *simulator.Params,
		stateHistories []*simulator.StateHistory,
	) general.Groupings {
		groupings := make(general.Groupings)
		var values []float64
		var ok bool
		for i, statePartitionIndex := range params.Get("state_partitions") {
			groupValue := stateHistories[int(
				params.GetIndex("grouping_partitions", i))].Values.At(
				0, int(params.GetIndex("grouping_value_indices", i)),
			)
			stateValue := stateHistories[int(statePartitionIndex)].Values.At(
				0, int(params.GetIndex("state_value_indices", i)),
			)
			values, ok = groupings[groupValue]
			if !ok {
				groupings[groupValue] = []float64{stateValue}
				continue
			}
			values = append(values, stateValue)
			groupings[groupValue] = values
		}
		return groupings
	}
}

// GetAcceptedValueGroups
func (g *GroupedStateTimeStorage) GetAcceptedValueGroups() []float64 {
	return []float64{}
}

// GetDefaults
func (g *GroupedStateTimeStorage) GetDefaults() []float64 {
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
