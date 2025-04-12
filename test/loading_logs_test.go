package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

func TestLoadingLogs(t *testing.T) {
	t.Run(
		"integration test: loading logs",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a log entries file
			storage, _ := analysis.NewStateTimeStorageFromJsonLogEntries("./data/test.log")

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "first_wiener_process"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)
		},
	)
}
