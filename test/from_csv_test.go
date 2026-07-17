package main

import (
	"os"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

// data/test_csv.csv is a fixed fixture rather than something the suite regenerates, and that
// is deliberate: because it never moves, it pins the on-disk CSV format. A change to the
// column layout, to how floats are rendered, or to how rows map onto partitions shows up here
// as a failure, rather than as a quietly-rewritten file that still round-trips against itself.
// Reading it back is the assertion, so it has to actually be asserted.
func TestFromCsv(t *testing.T) {
	t.Run(
		"integration test: from CSV",
		func(t *testing.T) {
			// The header is checked by hand because NewStateTimeStorageFromCsv skips it and
			// indexes columns by position — nothing it returns can witness a renamed or
			// reordered column, so without this the header could drift unnoticed.
			header, err := os.ReadFile("data/test_csv.csv")
			if err != nil {
				t.Fatalf("the fixture did not open: %v", err)
			}
			line, _, _ := strings.Cut(string(header), "\n")
			if got, want := line, "time,0,1,2,3"; got != want {
				t.Errorf("got header %q, want %q", got, want)
			}

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

			values := storage.GetValues("generated_data")
			if len(values) != 1001 {
				t.Fatalf("got %d rows, want 1001 (the initial state plus 1000)", len(values))
			}
			if len(values[0]) != 4 {
				t.Fatalf("got a width of %d, want 4", len(values[0]))
			}

			// Times come from column 0, in order.
			times := storage.GetTimes()
			if len(times) != 1001 || times[0] != 0 || times[len(times)-1] != 1000 {
				t.Errorf("got %d times spanning %v to %v, want 1001 spanning 0 to 1000",
					len(times), times[0], times[len(times)-1])
			}

			// Values read back at full precision, which is what catches a change in how floats
			// are written or parsed rather than merely in the shape of a row.
			if got, want := values[15][0], 0.57021; got != want {
				t.Errorf("generated_data at t=15: got %v, want %v", got, want)
			}
			if got, want := values[1000][2], -10.847508; got != want {
				t.Errorf("generated_data at t=1000: got %v, want %v", got, want)
			}

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "generated_data"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs); plot == nil {
				t.Error("expected a plot from the loaded fixture")
			}
		},
	)
}
