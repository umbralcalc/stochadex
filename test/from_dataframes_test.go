package main

import (
	"os"
	"slices"
	"testing"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// data/test_df.csv is a fixed fixture rather than something the suite regenerates, and that is
// deliberate: because it never moves, it pins the on-disk CSV format. A change to the column
// layout or to how the time column is rendered shows up here as a failure, rather than as a
// quietly-rewritten file that still round-trips against itself. Reading it back is the
// assertion, so it has to actually be asserted.
func TestFromDataframes(t *testing.T) {
	t.Run(
		"integration test: from dataframes",
		func(t *testing.T) {
			// Create a dataframe from the file
			file, err := os.Open("data/test_df.csv")
			if err != nil {
				t.Fatalf("the fixture did not open: %v", err)
			}
			defer file.Close()
			df := dataframe.ReadCSV(file)

			// The columns are read by name, so a renamed or dropped one silently yields a
			// partition of NaNs rather than an error.
			if got, want := df.Names(), []string{"time", "0", "1", "2", "3"}; !slices.Equal(got, want) {
				t.Fatalf("got columns %v, want %v", got, want)
			}

			// Create new state time storage for the data
			storage := simulator.NewStateTimeStorage()

			// Add the dataframe data to the storage as a partition
			analysis.SetPartitionFromDataFrame(storage, "poisson_data", df, true)

			values := storage.GetValues("poisson_data")
			if len(values) != 1001 {
				t.Fatalf("got %d rows, want 1001 (the initial state plus 1000)", len(values))
			}
			if len(values[0]) != 4 {
				t.Fatalf("got a width of %d, want 4", len(values[0]))
			}

			// The times are UNIX timestamps a thousand seconds apart, taken from the time
			// column because overwriteTime is set.
			times := storage.GetTimes()
			if len(times) != 1001 || times[0] != 1667980544.0 ||
				times[len(times)-1] != 1668980544.0 {
				t.Errorf("got %d times spanning %v to %v, want 1001 spanning 1667980544 to 1668980544",
					len(times), times[0], times[len(times)-1])
			}

			// Values read back at full precision, which is what catches a change in how the
			// columns are parsed rather than merely in the shape of a row.
			if got, want := values[15][2], 13.0; got != want {
				t.Errorf("poisson_data column 2 at row 15: got %v, want %v", got, want)
			}
			if got, want := values[1000][0], 708.0; got != want {
				t.Errorf("poisson_data column 0 at row 1000: got %v, want %v", got, want)
			}

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
