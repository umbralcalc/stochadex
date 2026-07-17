package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPostgresWritingAndQuerying(t *testing.T) {
	t.Run(
		"integration test: postgres writing and querying",
		func(t *testing.T) {
			// Create a simulator.StateTimeStorage from a simulation run
			storage := analysis.NewStateTimeStorageFromPartitions(
				// Instantiate the desired simulation state partitions
				[]*simulator.PartitionConfig{
					{
						Name: "gamma_compound_poisson",
						Iteration: &continuous.CompoundPoissonProcessIteration{
							JumpDist: &continuous.GammaJumpDistribution{},
						},
						Params: simulator.NewParams(map[string][]float64{
							"rates":        {0.5, 1.0, 0.8},
							"gamma_alphas": {1.0, 2.5, 3.0},
							"gamma_betas":  {2.0, 1.0, 4.1},
						}),
						InitStateValues:   []float64{0.0, 0.0, 0.0},
						StateHistoryDepth: 1,
						Seed:              5678,
					},
					{
						Name:      "group_numbers",
						Iteration: &discrete.BinomialObservationProcessIteration{},
						Params: simulator.NewParams(map[string][]float64{
							"observed_values":                 {5, 5, 5},
							"state_value_observation_probs":   {0.1, 0.4, 0.7},
							"state_value_observation_indices": {0, 1, 2},
						}),
						InitStateValues:   []float64{0.0, 0.0, 0.0},
						StateHistoryDepth: 1,
						Seed:              321,
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

			// Specify the Postgres DB configuration
			db := &analysis.PostgresDb{
				User:      "stochadexuser",
				Password:  "stochadexpassword",
				Dbname:    "stochadexdb",
				TableName: "testnumbers",
			}

			// Write the data to the configured Postgres DB
			analysis.WriteStateTimeStorageToPostgresDb(db, storage)

			// Create a simulator.StateTimeStorage using data from a DB table
			storage, err := analysis.NewStateTimeStorageFromPostgresDb(
				db,
				[]string{"gamma_compound_poisson", "group_numbers"},
				0.0,
				1000.0,
			)
			if err != nil {
				t.Fatalf("the data did not read back from the DB: %v", err)
			}

			// Read back what was just written. The table keys on (partition_name, time), so a
			// re-run overwrites rather than accumulates and these counts stay put.
			jumps := storage.GetValues("gamma_compound_poisson")
			groups := storage.GetValues("group_numbers")
			if len(jumps) != 1001 || len(groups) != 1001 {
				t.Fatalf("got %d and %d rows, want 1001 each (the initial state plus 1000)",
					len(jumps), len(groups))
			}
			if len(jumps[0]) != 3 || len(groups[0]) != 3 {
				t.Fatalf("got widths %d and %d, want 3 and 3", len(jumps[0]), len(groups[0]))
			}
			times := storage.GetTimes()
			if len(times) != 1001 || times[0] != 0 || times[len(times)-1] != 1000 {
				t.Errorf("got %d times spanning %v to %v, want 1001 spanning 0 to 1000",
					len(times), times[0], times[len(times)-1])
			}

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "gamma_compound_poisson"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			if plot := analysis.NewScatterPlotFromPartition(storage, xRef, yRefs); plot == nil {
				t.Error("expected a plot from the data read back from the DB")
			}
		},
	)
}
