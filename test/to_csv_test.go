package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestToCsv(t *testing.T) {
	t.Run(
		"integration test: to CSV",
		func(t *testing.T) {
			// Written to a temporary path, not to ./data/test_csv.csv. That file is a
			// committed fixture the reading tests pin the format against, and writing to it
			// here coupled the tests by execution order — the readers only saw the fixture
			// rather than this output because Go happens to order test files alphabetically,
			// so f comes before t.
			csvFile := filepath.Join(t.TempDir(), "test_csv.csv")

			// Create a simulator.StateTimeStorage from a simulation run
			storage := analysis.NewStateTimeStorageFromPartitions(
				// Instantiate the desired simulation state partitions
				[]*simulator.PartitionConfig{
					{
						Name: "generated_data",
						Iteration: &inference.DataGenerationIteration{
							Likelihood: &inference.NormalLikelihoodDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"mean":              {1.8, 5.0, -7.3, 2.2},
							"covariance_matrix": {2.5, 0.0, 0.0, 0.0, 0.0, 9.0, 0.0, 0.0, 0.0, 0.0, 4.4, 0.0, 0.0, 0.0, 0.0, 17.0},
						}),
						InitStateValues:   []float64{1.3, 8.3, -4.9, 1.6},
						StateHistoryDepth: 1,
						Seed:              1234567,
					},
				},
				// Decide when should we stop the simulation
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 1000,
				},
				// Decide how time should evolve
				&simulator.ConstantTimestepFunction{
					Stepsize: 1.0,
				},
				// Input the initial time
				0.0,
			)

			// Retrieve a dataframe representing the data in one partition
			df := analysis.GetDataFrameFromPartition(storage, "generated_data")

			// Save the dataframe as a CSV for later
			file, err := os.Create(csvFile)
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
			written, err := analysis.NewStateTimeStorageFromCsv(
				csvFile,
				0,
				map[string][]int{"generated_data": {1, 2, 3, 4}},
				true,
			)
			if err != nil {
				t.Fatalf("the CSV did not read back: %v", err)
			}
			values := written.GetValues("generated_data")
			if len(values) != 1001 {
				t.Fatalf("got %d rows, want 1001 (the initial state plus 1000)", len(values))
			}
			if len(values[0]) != 4 {
				t.Fatalf("got a width of %d, want 4", len(values[0]))
			}
			times := written.GetTimes()
			if len(times) != 1001 || times[0] != 0 || times[len(times)-1] != 1000 {
				t.Errorf("got %d times spanning %v to %v, want 1001 spanning 0 to 1000",
					len(times), times[0], times[len(times)-1])
			}

			// The initial state survives the round trip exactly, which is the one row whose
			// value does not depend on the RNG.
			if got, want := values[0][1], 8.3; got != want {
				t.Errorf("generated_data at t=0: got %v, want %v", got, want)
			}

			// Display the dataframe
			fmt.Println(df)
		},
	)
}
