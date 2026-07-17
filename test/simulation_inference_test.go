package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestSimulationInference(t *testing.T) {
	t.Run(
		"integration test: simulation inference",
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

			// Everything below infers from this data, so the fixture has to have landed before
			// any of it means anything.
			data := storage.GetValues("generated_data")
			if len(data) != 1001 || len(data[0]) != 4 {
				t.Fatalf("got %d rows of width %d, want 1001 of width 4", len(data), len(data[0]))
			}
			dataTimes := storage.GetTimes()
			if dataTimes[0] != 0 || dataTimes[len(dataTimes)-1] != 1000 {
				t.Fatalf("got times spanning %v to %v, want 0 to 1000",
					dataTimes[0], dataTimes[len(dataTimes)-1])
			}
			if got, want := data[15][0], 0.57021; got != want {
				t.Fatalf("generated_data at t=15: got %v, want %v", got, want)
			}

			// Configure a partition for computing the exponentially-weighted rolling mean
			meanPartition := analysis.NewVectorMeanPartition(
				analysis.AppliedAggregation{
					Name:   "mean",
					Data:   analysis.DataRef{PartitionName: "generated_data"},
					Kernel: &kernels.ConstantIntegrationKernel{},
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
					Kernel:       &kernels.ConstantIntegrationKernel{},
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

			// Configure a 'simulation' to infer the posterior samples of target parameters
			simulation := &simulator.PartitionConfig{
				Name: "regenerated_data",
				Iteration: &inference.DataGenerationIteration{
					Likelihood: &inference.NormalLikelihoodDistribution{},
				},
				Params: simulator.NewParams(map[string][]float64{
					"covariance_matrix": {2.5, 0.0, 0.0, 0.0, 0.0, 9.0, 0.0, 0.0, 0.0, 0.0, 4.4, 0.0, 0.0, 0.0, 0.0, 17.0},
				}),
				InitStateValues:   []float64{0.0, 0.0, 0.0, 0.0},
				StateHistoryDepth: 1,
				Seed:              123,
			}

			// Configure some partitions which collectively estimate and sample from the posterior
			// distribution over the target parameters (the rolling mean vector in this example)
			partitions := analysis.NewPosteriorEstimationPartitions(
				analysis.AppliedPosteriorEstimation{
					LogNorm: analysis.PosteriorLogNorm{
						Name:    "posterior_log_norm",
						Default: 0.0,
					},
					Mean: analysis.PosteriorMean{
						Name:    "posterior_mean",
						Default: []float64{0.0, 0.0, 0.0, 0.0},
					},
					Covariance: analysis.PosteriorCovariance{
						Name:    "posterior_cov",
						Default: []float64{5.0, 0.0, 0.0, 0.0, 0.0, 5.0, 0.0, 0.0, 0.0, 0.0, 5.0, 0.0, 0.0, 0.0, 0.0, 5.0},
					},
					Sampler: analysis.PosteriorSampler{
						Name:    "posterior_sampler",
						Default: []float64{0.0, 0.0, 0.0, 0.0},
						Distribution: analysis.ParameterisedModel{
							Likelihood: &inference.NormalLikelihoodDistribution{
								AllowDefaultCovarianceFallback: true,
							},
							Params: simulator.NewParams(map[string][]float64{
								"default_covariance": {5.0, 0.0, 0.0, 0.0, 0.0, 5.0, 0.0, 0.0, 0.0, 0.0, 5.0, 0.0, 0.0, 0.0, 0.0, 5.0},
								"cov_burn_in_steps":  {300},
							}),
							ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
								"mean":              {Upstream: "posterior_mean"},
								"covariance_matrix": {Upstream: "posterior_cov"},
							},
						},
					},
					Comparison: analysis.AppliedLikelihoodComparison{
						Name:  "loglikelihood",
						Model: model,
						Data:  analysis.DataRef{PartitionName: "regenerated_data"},
						Window: analysis.WindowedPartitions{
							Partitions: []analysis.WindowedPartition{{
								Partition: simulation,
								OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
									"mean": {Upstream: "posterior_sampler"},
								},
							}},
							Data: []analysis.DataRef{
								{PartitionName: "mean"},
								{PartitionName: "variance"},
							},
							Depth: 200,
						},
						WindowDataHistoryDepth: map[string]int{
							"mean":     200,
							"variance": 200,
						},
					},
					PastDiscount: 1.0,
					MemoryDepth:  300,
					Seed:         1234,
				},
				storage,
			)

			// Run and add the likelihood comparison partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				partitions,
				map[string]int{"mean": 200, "variance": 200},
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the likelihood partition plotting data for the y-axis
			yRefs := []analysis.DataRef{{
				PartitionName: "loglikelihood",
				ValueIndices:  []int{12},
			}}

			// Create a line plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil); plot == nil {
				t.Error("expected a line plot")
			}

			// Reference the posterior samples plotting data for the y-axis
			yRefs = []analysis.DataRef{{PartitionName: "posterior_sampler"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs); plot == nil {
				t.Error("expected a scatter plot")
			}

			// Reference the rolling mean plotting data (the target params) for the y-axis
			yRefs = []analysis.DataRef{{PartitionName: "mean"}}

			// Create a line plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil); plot == nil {
				t.Error("expected a line plot")
			}
		},
	)
}
