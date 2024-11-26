package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NewGroupedAggregationPartition creates a new PartitionConfig for a
// grouped aggregation.
func NewGroupedAggregationPartition(
	name string,
	aggregation func(
		defaultValues []float64,
		outputIndexByGroup map[string]int,
		groupings map[string][]float64,
		weightings map[string][]float64,
	) []float64,
	storage *GroupedStateTimeStorage,
) *simulator.PartitionConfig {
	defaults := storage.GetDefaults()
	acceptGroups := storage.GetAcceptedValueGroups()
	params := simulator.NewParams(map[string][]float64{
		"state_value_indices":    {},
		"grouping_value_indices": {},
		"accepted_value_groups":  acceptGroups,
	})
	var initStateValues []float64
	if defaults != nil {
		params.Set("default_values", defaults)
		initStateValues = defaults
	} else {
		initStateValues = make([]float64, 0)
		for range acceptGroups {
			initStateValues = append(initStateValues, 0.0)
		}
	}
	return &simulator.PartitionConfig{
		Name: name,
		Iteration: &general.ValuesGroupedAggregationIteration{
			Aggregation: aggregation,
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

// NewAggregationPartition creates a creates a new PartitionConfig for a
// instantaneous, weighted and/or windowed aggregation.
func NewAggregationPartition(
	name string,
	storage *GroupedStateTimeStorage,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{}
}
