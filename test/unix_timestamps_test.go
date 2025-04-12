package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

func TestUnixTimestamps(t *testing.T) {
	t.Run(
		"integration test: unix timestamps",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a simulation run
			storage := analysis.NewStateTimeStorageFromPartitions(
				// Instantiate the desired simulation state partitions
				[]*simulator.PartitionConfig{{
					Name:      "test_partition",
					Iteration: &continuous.WienerProcessIteration{},
					Params: simulator.NewParams(map[string][]float64{
						"variances": {1.0, 2.0, 3.0, 4.0},
					}),
					InitStateValues:   []float64{0.0, 0.0, 0.0, 0.0},
					StateHistoryDepth: 1,
					Seed:              12345,
				}},
				// Decide when should we stop the simulation
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				// Decide how time should evolve
				&simulator.ConstantTimestepFunction{
					Stepsize: 1000.0,
				},
				// Input the initial time
				1667980544.0,
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "test_partition"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			scatter := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)

			// Display date-time strings when the time is a UNIX timestamp
			scatter.SetGlobalOptions(charts.WithXAxisOpts(opts.XAxis{Type: "time"}))
		},
	)
}
