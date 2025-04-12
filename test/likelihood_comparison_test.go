package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestLikelihoodComparison(t *testing.T) {
	t.Run(
		"integration test: likelihood comparison",
		func(t *testing.T) {
			// Read in CSV data directly into a simulator.StateTimeStorage
			storage, _ := analysis.NewStateTimeStorageFromCsv(
				"data/test_csv.csv",
				0,
				map[string][]int{"generated_data": {1, 2, 3, 4}},
				true,
			)

			// Configure a partition for computing the exponentially-weighted rolling mean
			meanPartition := analysis.NewVectorMeanPartition(
				analysis.AppliedAggregation{
					Name:   "mean",
					Data:   analysis.DataRef{PartitionName: "generated_data"},
					Kernel: &kernels.ExponentialIntegrationKernel{},
				},
				storage,
			)

			// Set the timescale for the exponential weighting
			meanPartition.Params.Set("exponential_weighting_timescale", []float64{100.0})

			// Run and add the mean partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{meanPartition},
				map[string]int{"generated_data": 200},
			)

			// Configure a partition for computing the exponentially-weighted rolling variance
			variancePartition := analysis.NewVectorVariancePartition(
				analysis.DataRef{PartitionName: "mean"},
				analysis.AppliedAggregation{
					Name:         "variance",
					Data:         analysis.DataRef{PartitionName: "generated_data"},
					Kernel:       &kernels.ExponentialIntegrationKernel{},
					DefaultValue: 1.0,
				},
				storage,
			)

			// Set the timescale for the exponential weighting
			variancePartition.Params.Set("exponential_weighting_timescale", []float64{100.0})

			// Run and add the variance partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{variancePartition},
				map[string]int{"mean": 1, "generated_data": 200},
			)

			// Configure a model where the mean and variance are updated by the rolling estimators
			model := analysis.ParameterisedModel{
				Likelihood: &inference.NormalLikelihoodDistribution{},
				Params:     simulator.NewParams(make(map[string][]float64)),
				ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
					"mean":     {Upstream: "mean"},
					"variance": {Upstream: "variance"},
				},
			}

			// Configure a partition for computing the rolling likelihood comparison against the data
			comparisonPartition := analysis.NewLikelihoodComparisonPartition(
				analysis.AppliedLikelihoodComparison{
					Name:  "loglikelihood",
					Model: model,
					Data:  analysis.DataRef{PartitionName: "generated_data"},
					Window: analysis.WindowedPartitions{
						Data: []analysis.DataRef{
							{PartitionName: "generated_data"},
							{PartitionName: "mean"},
							{PartitionName: "variance"},
						},
						Depth: 200,
					},
				},
				storage,
			)

			// Run and add the likelihood comparison partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{comparisonPartition},
				map[string]int{"mean": 200, "variance": 200, "generated_data": 200},
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{
				Plotting: &analysis.DataPlotting{
					IsTime:    true,
					TimeRange: &analysis.IndexRange{Lower: 200, Upper: 1000},
				},
			}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{
				PartitionName: "loglikelihood",
				ValueIndices:  []int{12},
				Plotting: &analysis.DataPlotting{
					TimeRange: &analysis.IndexRange{Lower: 200, Upper: 1000},
				},
			}}

			// Create a line plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil)
		},
	)
}
