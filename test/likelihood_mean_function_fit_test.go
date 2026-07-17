package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestLikelihoodMeanFunctionFit(t *testing.T) {
	t.Run(
		"integration test: likelihood mean function fit",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a log entries file
			storage, err := analysis.NewStateTimeStorageFromJsonLogEntries("./data/test.log")
			if err != nil {
				t.Fatalf("the fixture did not load: %v", err)
			}

			// The fit below runs against this data, so the fixture has to have landed before
			// any of it means anything.
			data := storage.GetValues("first_wiener_process")
			if len(data) != 201 || len(data[0]) != 4 {
				t.Fatalf("got %d entries of width %d, want 201 of width 4",
					len(data), len(data[0]))
			}
			if got, want := data[15][0], 1.22168049752109; got != want {
				t.Fatalf("first_wiener_process at t=15: got %v, want %v", got, want)
			}

			// Configure a partition to dynamically fit the mean of the data values
			fitPartition := analysis.NewLikelihoodMeanFunctionFitPartition(
				analysis.AppliedLikelihoodMeanFunctionFit{
					Name: "mean_fit",
					Model: analysis.ParameterisedModelWithGradient{
						Likelihood: &inference.NormalLikelihoodDistribution{},
						Params: simulator.NewParams(map[string][]float64{
							"variance": {1.0, 2.0, 3.0, 4.0},
						}),
					},
					Gradient: analysis.LikelihoodMeanGradient{
						Function: inference.MeanGradientFunc,
						Width:    4,
					},
					Data:              analysis.DataRef{PartitionName: "first_wiener_process"},
					Window:            analysis.WindowedPartitions{Depth: 10},
					LearningRate:      0.05,
					DescentIterations: 10,
				},
				storage,
			)

			// Run and add the mean fit partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{fitPartition},
				map[string]int{"first_wiener_process": 10},
			)

			// Specify the time range for the plot using indices
			timeRange := &analysis.IndexRange{Lower: 10, Upper: 199}

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{
				IsTime:    true,
				TimeRange: timeRange,
			}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{
				{
					PartitionName: "mean_fit",
					ValueIndices:  []int{0, 1, 2, 3},
					Plotting:      &analysis.DataPlotting{TimeRange: timeRange},
				},
				{
					PartitionName: "first_wiener_process",
					Plotting:      &analysis.DataPlotting{TimeRange: timeRange},
				},
			}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs); plot == nil {
				t.Error("expected a scatter plot")
			}
		},
	)
}
