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

// GetKernel retrieves the integration kernel used, returning the
// default of instantaneous (no window) if initially unset.
func (a *AppliedAggregation) GetKernel() kernels.IntegrationKernel {
	if a.Kernel == nil {
		return &kernels.InstantaneousIntegrationKernel{}
	}
	return a.Kernel
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
	for _, index := range applied.Data.GetValueIndices(storage.Storage) {
		stateValueIndices = append(stateValueIndices, float64(index))
	}
	params := simulator.NewParams(map[string][]float64{
		"float_precision":     {float64(storage.GetPrecision())},
		"state_value_indices": stateValueIndices,
	})
	paramsAsPartitions := map[string][]string{
		"state_partition": {applied.Data.PartitionName},
	}
	paramsFromUpstream := map[string]simulator.NamedUpstreamConfig{
		"latest_states": {Upstream: applied.Data.PartitionName},
	}
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
		groupingPartition := storage.GetGroupingPartition(tupIndex)
		paramsAsPartitions["grouping_partition_tupindex_"+strTupIndex] =
			[]string{groupingPartition}
		paramsFromUpstream["latest_groupings_tupindex_"+strTupIndex] =
			simulator.NamedUpstreamConfig{Upstream: groupingPartition}
	}
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: &general.ValuesGroupedAggregationIteration{
			Aggregation: aggregation,
			Kernel:      applied.GetKernel(),
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
	initStateValues := make([]float64, 0)
	appliedValueIndices := applied.Data.GetValueIndices(storage)
	for range appliedValueIndices {
		initStateValues = append(initStateValues, 0.0)
	}
	dataValuesIndices := make([]float64, 0)
	for _, index := range appliedValueIndices {
		dataValuesIndices = append(dataValuesIndices, float64(index))
	}
	params := simulator.NewParams(map[string][]float64{
		"data_values_indices": dataValuesIndices,
	})
	paramsAsPartitions := map[string][]string{
		"data_values_partition": {applied.Data.PartitionName},
	}
	paramsFromUpstream := map[string]simulator.NamedUpstreamConfig{
		"latest_data_values": {
			Upstream: applied.Data.PartitionName,
			Indices:  appliedValueIndices,
		},
	}
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: &general.ValuesFunctionVectorMeanIteration{
			Function: general.DataValuesFunction,
			Kernel:   applied.GetKernel(),
		},
		Params:             params,
		ParamsAsPartitions: paramsAsPartitions,
		ParamsFromUpstream: paramsFromUpstream,
		InitStateValues:    initStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}

// NewVectorVariancePartition creates a creates a new PartitionConfig to
// compute the vector variance of the referenced data partition.
func NewVectorVariancePartition(
	mean DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	initStateValues := make([]float64, 0)
	appliedValueIndices := applied.Data.GetValueIndices(storage)
	for range appliedValueIndices {
		initStateValues = append(initStateValues, 0.0)
	}
	dataValuesIndices := make([]float64, 0)
	for _, index := range appliedValueIndices {
		dataValuesIndices = append(dataValuesIndices, float64(index))
	}
	params := simulator.NewParams(map[string][]float64{
		"data_values_indices": dataValuesIndices,
	})
	paramsAsPartitions := map[string][]string{
		"data_values_partition": {applied.Data.PartitionName},
	}
	paramsFromUpstream := map[string]simulator.NamedUpstreamConfig{
		"mean": {
			Upstream: mean.PartitionName,
			Indices:  mean.GetValueIndices(storage),
		},
		"latest_data_values": {
			Upstream: applied.Data.PartitionName,
			Indices:  appliedValueIndices,
		},
	}
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: &general.ValuesFunctionVectorMeanIteration{
			Function: general.DataValuesVarianceFunction,
			Kernel:   applied.GetKernel(),
		},
		Params:             params,
		ParamsAsPartitions: paramsAsPartitions,
		ParamsFromUpstream: paramsFromUpstream,
		InitStateValues:    initStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}

// NewVectorCovariancePartition creates a creates a new PartitionConfig to
// compute the vector covariance matrix of the referenced data partition.
func NewVectorCovariancePartition(
	mean DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	initStateValues := make([]float64, 0)
	appliedValueIndices := applied.Data.GetValueIndices(storage)
	num := len(appliedValueIndices)
	for i := 0; i < num*num; i++ {
		initStateValues = append(initStateValues, 0.0)
	}
	dataValuesIndices := make([]float64, 0)
	for _, index := range appliedValueIndices {
		dataValuesIndices = append(dataValuesIndices, float64(index))
	}
	params := simulator.NewParams(map[string][]float64{
		"data_values_indices": dataValuesIndices,
	})
	paramsAsPartitions := map[string][]string{
		"data_values_partition": {applied.Data.PartitionName},
	}
	paramsFromUpstream := map[string]simulator.NamedUpstreamConfig{
		"mean": {
			Upstream: mean.PartitionName,
			Indices:  mean.GetValueIndices(storage),
		},
		"latest_data_values": {
			Upstream: applied.Data.PartitionName,
			Indices:  appliedValueIndices,
		},
	}
	return &simulator.PartitionConfig{
		Name: applied.Name,
		Iteration: &general.ValuesFunctionVectorCovarianceIteration{
			Function: general.DataValuesFunction,
			Kernel:   applied.GetKernel(),
		},
		Params:             params,
		ParamsAsPartitions: paramsAsPartitions,
		ParamsFromUpstream: paramsFromUpstream,
		InitStateValues:    initStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	}
}
