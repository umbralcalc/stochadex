package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPosteriorImportanceResampling(t *testing.T) {
	t.Run(
		"integration test: posterior importance resampling",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a simulation run
			storage := analysis.NewStateTimeStorageFromPartitions(
				// Instantiate the desired simulation state partitions
				[]*simulator.PartitionConfig{{
					Name:      "custom_partition",
					Iteration: &MyCustomIteration{},
					Params: simulator.NewParams(map[string][]float64{
						"n_values": {10, 14, 27},
						"p_values": {0.3, 0.8, 0.1},
					}),
					InitStateValues:   []float64{0.0, 0.0, 0.0},
					StateHistoryDepth: 1,
					Seed:              3421,
				}},
				// Decide when should we stop the simulation
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 1000,
				},
				// Decide how time should evolve
				&simulator.ConstantTimestepFunction{
					Stepsize: 1.0,
				},
				// Input the initial time
				0.0,
			)

			// Configure some partitions to estimate a t-kernel for the past values and use
			// this to weight resampling from them for future posterior samples
			kernelPartitions := analysis.NewPosteriorTKernelEstimationPartitions(
				analysis.AppliedPosteriorTKernelEstimation{
					Names: analysis.PosteriorTKernelEstimationNames{
						Updater: "updater",
						Sampler: "sampler",
					},
					Comparison: analysis.AppliedTKernelComparison{
						Name: "kernel",
						Model: analysis.ParameterisedTKernel{
							Data:              analysis.DataRef{PartitionName: "custom_partition"},
							Depth:             20,
							DegreesOfFreedom:  1,
							ScaleMatrixValues: []float64{2.0, 0.0, 0.0, 0.0, 2.0, 0.0, 0.0, 0.0, 2.0},
							TimeDeltaRanges: []general.TimeDeltaRange{{
								LowerDelta: 0.0,
								UpperDelta: 21.0,
							}},
						},
						Data: analysis.DataRef{PartitionName: "custom_partition"},
						Window: analysis.WindowedPartitions{
							Data:  []analysis.DataRef{{PartitionName: "custom_partition"}},
							Depth: 100,
						},
					},
					Defaults: analysis.PosteriorTKernelDefaults{
						Updater: []float64{2.0, 0.0, 0.0, 0.0, 2.0, 0.0, 0.0, 0.0, 2.0, 1.0},
						Sampler: []float64{1.0, 1.0, 1.0},
					},
					PastDiscount: 1.0,
					MemoryDepth:  100,
					Seed:         1234,
				},
				storage,
			)

			// Run and add the posterior t-kernel estimation partitions to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				kernelPartitions,
				map[string]int{"custom_partition": 120},
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{
				{PartitionName: "custom_partition"},
				{PartitionName: "sampler"},
			}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)
		},
	)
}
