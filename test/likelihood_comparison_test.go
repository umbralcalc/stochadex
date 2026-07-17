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
			storage, err := analysis.NewStateTimeStorageFromCsv(
				"data/test_csv.csv",
				0,
				map[string][]int{"generated_data": {1, 2, 3, 4}},
				true,
			)
			if err != nil {
				t.Fatalf("the fixture did not load: %v", err)
			}

			// The comparison below runs against this data, so the fixture has to have landed
			// before any of it means anything.
			data := storage.GetValues("generated_data")
			if len(data) != 1001 || len(data[0]) != 4 {
				t.Fatalf("got %d rows of width %d, want 1001 of width 4", len(data), len(data[0]))
			}
			dataTimes := storage.GetTimes()
			if dataTimes[0] != 0 || dataTimes[len(dataTimes)-1] != 1000 {
				t.Fatalf("got times spanning %v to %v, want 0 to 1000",
					dataTimes[0], dataTimes[len(dataTimes)-1])
			}
			if got, want := data[1000][2], -10.847508; got != want {
				t.Fatalf("generated_data at t=1000: got %v, want %v", got, want)
			}

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
			if plot := analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil); plot == nil {
				t.Error("expected a line plot")
			}
		},
	)
}
