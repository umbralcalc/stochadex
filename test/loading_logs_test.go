package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

// data/test.log is a fixed fixture rather than something the suite regenerates, and that is
// deliberate: because it never moves, it pins the on-disk log format. A change to the JSON
// keys, to how floats are rendered, or to how entries group into partitions shows up here as a
// failure, rather than as a quietly-rewritten file that still round-trips against itself.
// Reading it back is the assertion, so it has to actually be asserted — this test discarded
// the error and passed against an empty file.
func TestLoadingLogs(t *testing.T) {
	t.Run(
		"integration test: loading logs",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a log entries file
			storage, err := analysis.NewStateTimeStorageFromJsonLogEntries("./data/test.log")
			if err != nil {
				t.Fatalf("the fixture did not load: %v", err)
			}

			// The partition names come from the log's partition_name key, so a renamed or
			// dropped key loses them.
			first := storage.GetValues("first_wiener_process")
			second := storage.GetValues("second_wiener_process")
			if len(first) != 201 || len(second) != 201 {
				t.Fatalf("got %d and %d entries, want 201 each (the initial state plus 200)",
					len(first), len(second))
			}
			if len(first[0]) != 4 || len(second[0]) != 2 {
				t.Fatalf("got widths %d and %d, want 4 and 2", len(first[0]), len(second[0]))
			}

			// Times are the log's own, in order.
			times := storage.GetTimes()
			if len(times) != 201 || times[0] != 0 || times[len(times)-1] != 200 {
				t.Errorf("got %d times spanning %v to %v, want 201 spanning 0 to 200",
					len(times), times[0], times[len(times)-1])
			}

			// Values read back at full precision, which is what catches a change in how floats
			// are written or parsed rather than merely in the shape of an entry.
			if got, want := first[15][0], 1.22168049752109; got != want {
				t.Errorf("first_wiener_process at t=15: got %v, want %v", got, want)
			}
			if got, want := first[200][3], -42.7235171584224; got != want {
				t.Errorf("first_wiener_process at t=200: got %v, want %v", got, want)
			}

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "first_wiener_process"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs); plot == nil {
				t.Error("expected a plot from the loaded fixture")
			}
		},
	)
}
