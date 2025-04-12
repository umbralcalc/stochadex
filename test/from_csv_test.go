package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

func TestFromCsv(t *testing.T) {
	t.Run(
		"integration test: from CSV",
		func(t *testing.T) {
			// Read in CSV data directly into a simulator.StateTimeStorage
			storage, _ := analysis.NewStateTimeStorageFromCsv(
				"data/test_csv.csv",
				0,
				map[string][]int{"generated_data": {1, 2, 3, 4}},
				true,
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "generated_data"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)
		},
	)
}
