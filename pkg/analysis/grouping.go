package analysis

import (
	"fmt"
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

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
		return make(general.Groupings)
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

// GroupingConfig
type GroupingConfig struct {
	Data    DataRef
	GroupBy []DataRef
}

// KeyedGroup
type KeyedGroup struct {
	Key   string
	Group []float64
}

// roundToPrecision rounds floats to n decimal places.
func roundToPrecision(value float64, precision int) float64 {
	format := "%." + strconv.Itoa(precision) + "f"
	roundedValue, _ := strconv.ParseFloat(fmt.Sprintf(format, value), 64)
	return roundedValue
}

// floatTupleToKey converts a slice of floats to a string key with
// fixed precision for float values.
func floatTupleToKey(tuple []float64, precision int) string {
	key := ""
	for _, v := range tuple {
		rounded := roundToPrecision(v, precision)
		key += strconv.FormatFloat(rounded, 'f', precision, 64) + ","
	}
	return key
}
