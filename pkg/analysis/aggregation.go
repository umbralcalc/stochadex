package analysis

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AppliedAggregation describes how to aggregate a referenced dataset
// over time using customizable weighting kernels.
//
// This struct configures the aggregation process by specifying the source data,
// output partition name, weighting scheme, and handling of insufficient history.
// It serves as a blueprint for creating aggregation partitions in simulations.
//
// Mathematical Concept:
// Aggregations compute weighted averages over historical data:
//
//	A(t) = Σ w(t-s) * f(X(s)) / Σ w(t-s)
//
// where w(t-s) is the kernel weight, f(X(s)) is the source data, and the sum
// is over all historical samples s ≤ t.
//
// Fields:
//   - Name: Output partition name for the aggregated results
//   - Data: Reference to the source data partition and value indices
//   - Kernel: Integration kernel for time-based weighting (nil = instantaneous)
//   - DefaultValue: Fill value when insufficient history is available
//
// Related Types:
//   - See kernels.IntegrationKernel for available weighting schemes
//   - See DataRef for source data configuration
//   - See NewGroupedAggregationPartition for grouped aggregations
//
// Example:
//
//	aggregation := AppliedAggregation{
//	    Name: "rolling_mean",
//	    Data: DataRef{
//	        PartitionName: "prices",
//	        ValueIndices: []int{0, 1}, // Use first two price columns
//	    },
//	    Kernel: &kernels.ExponentialIntegrationKernel{},
//	    DefaultValue: 0.0,
//	}
type AppliedAggregation struct {
	Name         string
	Data         DataRef
	Kernel       kernels.IntegrationKernel
	DefaultValue float64
}

// GetKernel returns the configured integration kernel with automatic fallback.
//
// This method ensures that callers never need to handle nil kernels by providing
// a sensible default. The instantaneous kernel applies no time weighting,
// effectively using only the most recent value for aggregation.
//
// Returns:
//   - kernels.IntegrationKernel: The configured kernel, or InstantaneousIntegrationKernel if nil
//
// Usage:
//
//	kernel := aggregation.GetKernel()
//	// Safe to use kernel without nil checks
//
// Performance:
//   - O(1) time complexity
//   - No memory allocation for cached kernels
func (a *AppliedAggregation) GetKernel() kernels.IntegrationKernel {
	if a.Kernel == nil {
		return &kernels.InstantaneousIntegrationKernel{}
	}
	return a.Kernel
}

// NewGroupedAggregationPartition creates a partition that performs grouped
// aggregations over historical state values with customizable binning.
//
// This function creates a partition that aggregates data by grouping values
// into bins and applying custom aggregation functions within each group.
// It's particularly useful for computing statistics over value ranges or
// categorical data.
//
// Mathematical Concept:
// Grouped aggregations compute statistics within value bins:
//
//	G_i(t) = aggregate({X(s) : X(s) ∈ bin_i, s ≤ t})
//
// where bin_i represents a value range or category, and aggregate is the
// user-provided function (e.g., mean, sum, count).
//
// Parameters:
//   - aggregation: Function that computes aggregated values from grouped data.
//     Input parameters:
//   - defaultValues: Fill values for each group when no data is available
//   - outputIndexByGroup: Maps group names to output vector indices
//   - groupings: Maps group names to their historical values
//   - weightings: Maps group names to their time-based weights
//     Output: []float64 aggregated results ordered by accepted value groups
//   - applied: AppliedAggregation configuration specifying source data and kernel
//   - storage: GroupedStateTimeStorage containing group definitions and binning rules
//
// Returns:
//   - *PartitionConfig: Configured partition ready for simulation
//
// Example:
//
//	// Aggregate price data by volatility bins
//	config := NewGroupedAggregationPartition(
//	    func(defaults, indices, groups, weights map[string][]float64) []float64 {
//	        results := make([]float64, len(indices))
//	        for group, idx := range indices {
//	            values := groups[group]
//	            w := weights[group]
//	            // Compute weighted mean
//	            sum := 0.0
//	            totalWeight := 0.0
//	            for i, v := range values {
//	                sum += v * w[i]
//	                totalWeight += w[i]
//	            }
//	            if totalWeight > 0 {
//	                results[idx] = sum / totalWeight
//	            } else {
//	                results[idx] = defaults[idx]
//	            }
//	        }
//	        return results
//	    },
//	    AppliedAggregation{
//	        Name: "volatility_aggregates",
//	        Data: DataRef{PartitionName: "prices"},
//	        Kernel: &kernels.ExponentialIntegrationKernel{},
//	        DefaultValue: 0.0,
//	    },
//	    volatilityStorage,
//	)
//
// Performance Notes:
//   - O(n * m) time complexity where n is history depth, m is number of groups
//   - Memory usage scales with group count and history depth
//   - Efficient for moderate numbers of groups (< 1000)
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
	defaults := make([]float64, storage.GetAcceptedValueGroupsLength())
	for i := range defaults {
		defaults[i] = applied.DefaultValue
	}
	params.Set("default_values", defaults)
	for tupIndex := range storage.GetGroupTupleLength() {
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

// NewVectorMeanPartition creates a partition that computes rolling weighted means
// for each dimension of the referenced data.
//
// This function creates a partition that maintains running weighted averages
// over historical data using the specified integration kernel for time weighting.
// Each dimension of the source data is aggregated independently.
//
// Mathematical Concept:
// Vector mean aggregation computes:
//
//	μ_i(t) = Σ w(t-s) * X_i(s) / Σ w(t-s)
//
// where μ_i(t) is the mean for dimension i at time t, w(t-s) is the kernel weight,
// and X_i(s) is the value of dimension i at historical time s.
//
// Parameters:
//   - applied: AppliedAggregation specifying source data, kernel, and output name
//   - storage: StateTimeStorage containing the source data
//
// Returns:
//   - *PartitionConfig: Partition that outputs rolling weighted means
//
// Example:
//
//	// Compute exponentially weighted moving averages of price data
//	meanPartition := NewVectorMeanPartition(
//	    AppliedAggregation{
//	        Name: "price_ema",
//	        Data: DataRef{
//	            PartitionName: "prices",
//	            ValueIndices: []int{0, 1, 2}, // Use first 3 price dimensions
//	        },
//	        Kernel: &kernels.ExponentialIntegrationKernel{},
//	        DefaultValue: 100.0, // Initial price assumption
//	    },
//	    priceStorage,
//	)
//
// Performance:
//   - O(d * h) time complexity where d is data dimensions, h is history depth
//   - Memory usage: O(d) for output state
//   - Efficient for moderate dimensions (< 100)
func NewVectorMeanPartition(
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	appliedValueIndices := applied.Data.GetValueIndices(storage)
	initStateValues := make([]float64, len(appliedValueIndices))
	for i := range initStateValues {
		initStateValues[i] = applied.DefaultValue
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

// NewVectorVariancePartition constructs a PartitionConfig that computes the
// rolling windowed weighted variance per-index of the referenced data
// values. Provide the corresponding rolling mean via the mean DataRef.
func NewVectorVariancePartition(
	mean DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	appliedValueIndices := applied.Data.GetValueIndices(storage)
	initStateValues := make([]float64, len(appliedValueIndices))
	for i := range initStateValues {
		initStateValues[i] = applied.DefaultValue
	}
	dataValuesIndices := make([]float64, 0)
	for _, index := range appliedValueIndices {
		dataValuesIndices = append(dataValuesIndices, float64(index))
	}
	params := simulator.NewParams(map[string][]float64{
		"subtract_from_normalisation": {1},
		"data_values_indices":         dataValuesIndices,
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

// NewVectorCovariancePartition constructs a PartitionConfig that computes the
// rolling windowed weighted covariance matrix of the referenced data values.
// Provide the corresponding rolling mean via the mean DataRef.
func NewVectorCovariancePartition(
	mean DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	appliedValueIndices := applied.Data.GetValueIndices(storage)
	num := len(appliedValueIndices)
	initStateValues := make([]float64, num*num)
	for i := range num {
		for j := range num {
			switch i {
			case j:
				initStateValues[i+j] = applied.DefaultValue
			default:
				initStateValues[i+j] = 0.0
			}
		}
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
