package main

import (
	"os"
	"testing"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestFromDataframes(t *testing.T) {
	t.Run(
		"integration test: from dataframes",
		func(t *testing.T) {
			// Create a dataframe from the file
			file, _ := os.Open("data/test_df.csv")
			df := dataframe.ReadCSV(file)

			// Create new state time storage for the data
			storage := simulator.NewStateTimeStorage()

			// Add the dataframe data to the storage as a partition
			analysis.SetPartitionFromDataFrame(storage, "poisson_data", df, true)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "poisson_data"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			scatter := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)

			// Display date-time strings when the time is a UNIX timestamp
			scatter.SetGlobalOptions(charts.WithXAxisOpts(opts.XAxis{Type: "time"}))
		},
	)
}
