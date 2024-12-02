package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AppliedAggregation is the base configuration for an aggregation
// over a referenced dataset.
type AppliedAggregation struct {
	Name   string
	Data   DataRef
	Kernel kernels.IntegrationKernel
}

// NewGroupedAggregationPartition creates a new PartitionConfig for a
// grouped aggregation.
func NewGroupedAggregationPartition(
	aggregation func(
		defaultValues []float64,
		outputIndexByGroup map[string]int,
		groupings map[string][]float64,
		weightings map[string][]float64,
	) []float64,
	applied AppliedAggregation,
	storage *GroupedStateTimeStorage,
) *simulator.PartitionConfig {
	stateValueIndices := make([]float64, 0)
	for _, index := range applied.Data.ValueIndices {
		stateValueIndices = append(stateValueIndices, float64(index))
	}
	params := simulator.NewParams(map[string][]float64{
		"float_precision":     {float64(storage.GetPrecision())},
		"state_value_indices": stateValueIndices,
	})
	paramsAsPartitions := map[string][]string{
		"state_partition": {applied.Data.PartitionName},
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
		paramsAsPartitions["grouping_partition_tupindex_"+strTupIndex] =
			[]string{storage.GetGroupingPartition(tupIndex)}
	}
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: &general.ValuesGroupedAggregationIteration{
			Aggregation: aggregation,
			Kernel:      applied.Kernel,
		},
		Params:             params,
		ParamsAsPartitions: paramsAsPartitions,
		ParamsFromUpstream: paramsFromUpstream,
		InitStateValues:    defaults,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}

// NewVectorMeanPartition creates a creates a new PartitionConfig to
// compute the vector mean of the referenced data partition.
func NewVectorMeanPartition(
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{}
}

// NewVectorVariancePartition creates a creates a new PartitionConfig to
// compute the vector variance of the referenced data partition.
func NewVectorVariancePartition(
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{}
}

// NewVectorCovariancePartition creates a creates a new PartitionConfig to
// compute the vector covariance matrix of the referenced data partition.
func NewVectorCovariancePartition(
	mean DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	return &simulator.PartitionConfig{}
}
