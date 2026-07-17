package main

import (
	"os"
	"slices"
	"testing"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"gonum.org/v1/gonum/floats"
)

// Create a function for differencing of the Poisson data to run as a partition
func diffFunc(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	values := make([]float64, stateHistories[partitionIndex].StateWidth)
	floats.SubTo(values, params.Get("data"), stateHistories[int(
		params.GetIndex("partition", 0))].Values.RawRowView(0))
	return values
}

func TestRollingVectorAggregations(t *testing.T) {
	t.Run(
		"integration test: rolling vector aggregations",
		func(t *testing.T) {
			// Initialise a csv file reader
			file, err := os.Open("data/test_df.csv")
			if err != nil {
				t.Fatalf("the fixture did not open: %v", err)
			}
			defer file.Close()

			// Create a dataframe from the file
			df := dataframe.ReadCSV(file)

			// Create new state time storage for the data
			storage := simulator.NewStateTimeStorage()

			// Add the dataframe data to the storage as a partition
			analysis.SetPartitionFromDataFrame(storage, "poisson_data", df, true)

			// The aggregations below run over this data, so the fixture has to have landed
			// before any of it means anything — the columns are read by name and a renamed or
			// dropped one silently yields NaNs rather than an error.
			if got, want := df.Names(), []string{"time", "0", "1", "2", "3"}; !slices.Equal(got, want) {
				t.Fatalf("got columns %v, want %v", got, want)
			}
			data := storage.GetValues("poisson_data")
			if len(data) != 1001 || len(data[0]) != 4 {
				t.Fatalf("got %d rows of width %d, want 1001 of width 4", len(data), len(data[0]))
			}
			dataTimes := storage.GetTimes()
			if dataTimes[0] != 1667980544.0 || dataTimes[len(dataTimes)-1] != 1668980544.0 {
				t.Fatalf("got times spanning %v to %v, want 1667980544 to 1668980544",
					dataTimes[0], dataTimes[len(dataTimes)-1])
			}
			if got, want := data[1000][0], 708.0; got != want {
				t.Fatalf("poisson_data column 0 at row 1000: got %v, want %v", got, want)
			}

			// Configure a partition for differencing of the Poisson data
			diffPartition := &simulator.PartitionConfig{
				Name:      "diff_poisson_data",
				Iteration: &general.ValuesFunctionIteration{Function: diffFunc},
				Params:    simulator.NewParams(make(map[string][]float64)),
				ParamsAsPartitions: map[string][]string{
					"partition": {"poisson_data"},
				},
				ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
					"data": {Upstream: "poisson_data"},
				},
				InitStateValues:   []float64{0.0, 0.0, 0.0, 0.0},
				StateHistoryDepth: 1,
				Seed:              0,
			}

			// Run and add the differencing partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{diffPartition},
				map[string]int{"poisson_data": 1},
			)

			// Configure a partition for computing the exponentially-weighted rolling mean
			meanPartition := analysis.NewVectorMeanPartition(
				analysis.AppliedAggregation{
					Name:   "diffs_mean",
					Data:   analysis.DataRef{PartitionName: "diff_poisson_data"},
					Kernel: &kernels.ExponentialIntegrationKernel{},
				},
				storage,
			)

			// Set the timescale for the exponential weighting
			meanPartition.Params.Set("exponential_weighting_timescale", []float64{70000.0})

			// Run and add the mean partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{meanPartition},
				map[string]int{"diff_poisson_data": 100},
			)

			// Configure a partition for computing the exponentially-weighted rolling variance
			variancePartition := analysis.NewVectorVariancePartition(
				analysis.DataRef{PartitionName: "diffs_mean"},
				analysis.AppliedAggregation{
					Name:   "diffs_variance",
					Data:   analysis.DataRef{PartitionName: "diff_poisson_data"},
					Kernel: &kernels.ExponentialIntegrationKernel{},
				},
				storage,
			)

			// Set the timescale for the exponential weighting
			variancePartition.Params.Set("exponential_weighting_timescale", []float64{70000.0})

			// Run and add the variance partition to storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{variancePartition},
				map[string]int{"diffs_mean": 1, "diff_poisson_data": 100},
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{
				Plotting: &analysis.DataPlotting{
					IsTime:    true,
					TimeRange: &analysis.IndexRange{Lower: 100, Upper: 1000},
				},
			}

			// Reference the mean partition plotting data for the y-axis
			yRefs := []analysis.DataRef{{
				PartitionName: "diffs_mean",
				Plotting: &analysis.DataPlotting{
					TimeRange: &analysis.IndexRange{Lower: 100, Upper: 1000},
				},
			}}

			// Create a line plot from partitions in a simulator.StateTimeStorage
			line := analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil)

			// Display date-time strings when the time is a UNIX timestamp
			line.SetGlobalOptions(charts.WithXAxisOpts(opts.XAxis{Type: "time"}))

			// Reference the variance partition plotting data for the y-axis
			yRefs = []analysis.DataRef{{
				PartitionName: "diffs_variance",
				Plotting: &analysis.DataPlotting{
					TimeRange: &analysis.IndexRange{Lower: 100, Upper: 1000},
				},
			}}

			// Create another line plot from partitions in a simulator.StateTimeStorage
			line = analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil)

			// Display date-time strings when the time is a UNIX timestamp
			line.SetGlobalOptions(charts.WithXAxisOpts(opts.XAxis{Type: "time"}))
		},
	)
}
