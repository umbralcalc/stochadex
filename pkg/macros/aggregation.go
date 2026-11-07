package macros

import (
	"github.com/umbralcalc/stochadex/pkg/analysis"
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
//   - See analysis.DataRef for source data configuration
//   - See NewGroupedAggregationPartition for grouped aggregations
//
// Example:
//
//	aggregation := AppliedAggregation{
//	    Name: "rolling_mean",
//	    Data: analysis.DataRef{
//	        PartitionName: "prices",
//	        ValueIndices: []int{0, 1}, // Use first two price columns
//	    },
//	    Kernel: &kernels.ExponentialIntegrationKernel{},
//	    DefaultValue: 0.0,
//	}
type AppliedAggregation struct {
	Name         string
	Data         analysis.DataRef
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
//   - storage: analysis.GroupedStateTimeStorage containing group definitions and binning rules
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
//	        Data: analysis.DataRef{PartitionName: "prices"},
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
	storage *analysis.GroupedStateTimeStorage,
) *simulator.PartitionConfig {
	groupings := make([]AggregationGrouping, storage.GetGroupTupleLength())
	for k := range groupings {
		groupings[k] = AggregationGrouping{
			Partition:      storage.GetGroupingPartition(k),
			ValueIndices:   storage.GetGroupingValueIndices(k),
			AcceptedValues: storage.GetAcceptedValueGroups(k),
		}
	}
	return NewGroupedAggregationLayout(
		aggregationRef(applied.Data, storage.Storage), groupings, storage.GetPrecision(),
		storage.GetAcceptedValueGroupsLength(), applied.DefaultValue,
	).partition(applied.Name, &general.ValuesGroupedAggregationIteration{
		Aggregation: aggregation,
		Kernel:      applied.GetKernel(),
	})
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
//	        Data: analysis.DataRef{
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
	return NewVectorMeanLayout(aggregationRef(applied.Data, storage), applied.DefaultValue).
		partition(applied.Name, &general.ValuesFunctionVectorMeanIteration{
			Function: general.DataValuesFunction,
			Kernel:   applied.GetKernel(),
		})
}

// NewVectorCumulativeMeanPartition constructs a PartitionConfig whose state is
// the running mean of the referenced data values over every row the run sees:
// the data partition's initial row, which is the partition's own initial
// state, and each step's row after it, weighted equally. Unlike
// NewVectorMeanPartition it reads no history, so a step costs O(d) however
// long the data; applied.Kernel and applied.DefaultValue are not used. It
// starts from the data partition's initial state (InitStateFromUpstream).
func NewVectorCumulativeMeanPartition(
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	return NewVectorCumulativeMeanLayout(aggregationRef(applied.Data, storage)).
		partition(applied.Name, &general.ValuesFunctionCumulativeMeanIteration{
			Function: general.DataValuesFunction,
		})
}

// NewVectorVariancePartition constructs a PartitionConfig that computes the
// rolling windowed weighted variance per-index of the referenced data
// values. Provide the corresponding rolling mean via the mean analysis.DataRef.
func NewVectorVariancePartition(
	mean analysis.DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	return NewVectorVarianceLayout(aggregationRef(mean, storage),
		aggregationRef(applied.Data, storage), applied.DefaultValue,
	).partition(applied.Name, &general.ValuesFunctionVectorMeanIteration{
		Function: general.DataValuesVarianceFunction,
		Kernel:   applied.GetKernel(),
	})
}

// NewVectorCovariancePartition constructs a PartitionConfig that computes the
// rolling windowed weighted covariance matrix of the referenced data values.
// Provide the corresponding rolling mean via the mean analysis.DataRef.
func NewVectorCovariancePartition(
	mean analysis.DataRef,
	applied AppliedAggregation,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	return NewVectorCovarianceLayout(aggregationRef(mean, storage),
		aggregationRef(applied.Data, storage), applied.DefaultValue,
	).partition(applied.Name, &general.ValuesFunctionVectorCovarianceIteration{
		Function: general.DataValuesFunction,
		Kernel:   applied.GetKernel(),
	})
}
