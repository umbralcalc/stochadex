package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
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
	kernel kernels.IntegrationKernel,
	storage *GroupedStateTimeStorage,
) *simulator.PartitionConfig {
	params := simulator.NewParams(map[string][]float64{
		"float_precision":     {float64(storage.GetPrecision())},
		"state_value_indices": storage.GetStateValueIndices(),
	})
	paramsAsPartitions := map[string][]string{
		"state_partitions": storage.GetStatePartitions(),
	}
	paramsFromUpstream := map[string]simulator.NamedUpstreamConfig{}
	defaults := storage.GetDefaults()
	params.Set("default_values", defaults)
	for tupIndex := 0; tupIndex < storage.GetGroupTupleLength(); tupIndex++ {
		strTupIndex := strconv.Itoa(tupIndex)
		params.Set(
			"grouping_value_indices_tupindex_"+strTupIndex,
			storage.GetGroupingValueIndices(tupIndex),
		)
		params.Set(
			"accepted_value_group_tupindex_"+strTupIndex,
			storage.GetAcceptedValueGroups(tupIndex),
		)
		paramsAsPartitions["grouping_partitions_tupindex_"+strTupIndex] =
			storage.GetGroupingPartitions(tupIndex)
	}
	return &simulator.PartitionConfig{
		Name: name,
		Iteration: &general.ValuesGroupedAggregationIteration{
			Aggregation: aggregation,
			Kernel:      kernel,
		},
		Params:             params,
		ParamsAsPartitions: paramsAsPartitions,
		ParamsFromUpstream: paramsFromUpstream,
		InitStateValues:    defaults,
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
