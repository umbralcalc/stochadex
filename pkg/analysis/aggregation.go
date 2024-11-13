package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AggDataRef
type AggDataRef struct {
	PartitionName string
	ValueIndex    int
}

// GroupedAggregationConfig
type GroupedAggregationConfig struct {
	Name         string
	Data         AggDataRef
	GroupBy      []AggDataRef
	AcceptGroups []float64
	Defaults     []float64 // optional
	AggFunction  func(
		currentAggValue float64,
		nextValueCount int,
		nextValue float64,
	) float64
}

// ValuesFunction
func (g *GroupedAggregationConfig) ValuesFunction() func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []general.GroupStateValue {
	return func(
		params *simulator.Params,
		partitionIndex int,
		stateHistories []*simulator.StateHistory,
		timestepsHistory *simulator.CumulativeTimestepsHistory,
	) []general.GroupStateValue {
		return []general.GroupStateValue{}
	}
}

// NewGroupedAggregationPartition
func NewGroupedAggregationPartition(
	config *GroupedAggregationConfig,
) *simulator.PartitionConfig {
	params := simulator.NewParams(map[string][]float64{
		"state_value_indices":    {},
		"grouping_value_indices": {},
		"accepted_value_groups":  config.AcceptGroups,
	})
	var initStateValues []float64
	if config.Defaults != nil {
		params.Set("default_values", config.Defaults)
		initStateValues = config.Defaults
	} else {
		initStateValues = make([]float64, 0)
		for range config.AcceptGroups {
			initStateValues = append(initStateValues, 0.0)
		}
	}
	return &simulator.PartitionConfig{
		Name: config.Name,
		Iteration: &general.ValuesGroupedAggregationIteration{
			ValuesFunction: config.ValuesFunction(),
			AggFunction:    config.AggFunction,
		},
		Params: params,
		ParamsAsPartitions: map[string][]string{
			"state_partitions":    {},
			"grouping_partitions": {},
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{},
		InitStateValues:    initStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}

// WeightedWindowedAggregationConfig
type WeightedWindowedAggregationConfig struct {
	Name string
	Data []AggDataRef
}
