package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gota/gota/dataframe"
	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestToDataframes(t *testing.T) {
	t.Run(
		"integration test: to dataframes",
		func(t *testing.T) {
			// Written to a temporary path, not to ./data/test_df.csv — see the note in
			// to_csv_test.go.
			dfFile := filepath.Join(t.TempDir(), "test_df.csv")

			// Create a simulator.StateTimeStorage from a simulation run
			storage := analysis.NewStateTimeStorageFromPartitions(
				// Instantiate the desired simulation state partitions
				[]*simulator.PartitionConfig{
					{
						Name:      "poisson_data",
						Iteration: &discrete.PoissonProcessIteration{},
						Params: simulator.NewParams(map[string][]float64{
							"rates": {0.002, 0.001, 0.004, 0.001},
						}),
						InitStateValues:   []float64{0.0, 0.0, 0.0, 0.0},
						StateHistoryDepth: 1,
						Seed:              123,
					},
				},
				// Decide when should we stop the simulation
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 1000,
				},
				// Decide how time should evolve
				&simulator.ConstantTimestepFunction{
					Stepsize: 1000.0,
				},
				// Input the initial time
				1667980544.0,
			)

			// Retrieve a dataframe representing the data in one partition
			df := analysis.GetDataFrameFromPartition(storage, "poisson_data")

			// Save the dataframe as a csv for later
			file, err := os.Create(dfFile)
			if err != nil {
				t.Fatalf("could not create the output file: %v", err)
			}
			if err := df.WriteCSV(file); err != nil {
				t.Fatalf("could not write the dataframe as CSV: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("could not close the output file: %v", err)
			}

			// Read it back. Nothing else looks at this output now, so without this the test
			// would pass whether or not a single row was written.
			readBack, err := os.Open(dfFile)
			if err != nil {
				t.Fatalf("the CSV did not open: %v", err)
			}
			defer readBack.Close()
			written := simulator.NewStateTimeStorage()
			analysis.SetPartitionFromDataFrame(
				written, "poisson_data", dataframe.ReadCSV(readBack), true)

			values := written.GetValues("poisson_data")
			if len(values) != 1001 {
				t.Fatalf("got %d rows, want 1001 (the initial state plus 1000)", len(values))
			}
			if len(values[0]) != 4 {
				t.Fatalf("got a width of %d, want 4", len(values[0]))
			}

			// The timestamps are the ones configured above, so they pin the round trip
			// independently of the RNG.
			times := written.GetTimes()
			if len(times) != 1001 || times[0] != 1667980544.0 ||
				times[len(times)-1] != 1668980544.0 {
				t.Errorf("got %d times spanning %v to %v, want 1001 spanning 1667980544 to 1668980544",
					len(times), times[0], times[len(times)-1])
			}

			// A Poisson counter starts at zero and only ever increases.
			if got := values[0]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
				t.Errorf("got an initial state of %v, want all zeros", got)
			}
			for i := range values[0] {
				if values[1000][i] < values[0][i] {
					t.Errorf("counter %d went backwards: %v then %v",
						i, values[0][i], values[1000][i])
				}
			}

			// Display the dataframe
			fmt.Println(df)
		},
	)
}
