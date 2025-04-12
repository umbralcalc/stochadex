package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestToDataframes(t *testing.T) {
	t.Run(
		"integration test: to dataframes",
		func(t *testing.T) {
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
			file, _ := os.Create("data/test_df.csv")
			df.WriteCSV(file)

			// Display the dataframe
			fmt.Println(df)
		},
	)
}
