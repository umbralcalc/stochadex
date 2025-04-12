package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestToCsv(t *testing.T) {
	t.Run(
		"integration test: to CSV",
		func(t *testing.T) {
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
			file, _ := os.Create("data/test_csv.csv")
			df.WriteCSV(file)

			// Display the dataframe
			fmt.Println(df)
		},
	)
}
