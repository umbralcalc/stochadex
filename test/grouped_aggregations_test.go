package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestGroupedAggregations(t *testing.T) {
	t.Run(
		"integration test: grouped aggregations",
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

			// Apply a grouping to the data in order to get a 'grouped storage'
			groupedStorage := analysis.NewGroupedStateTimeStorage(
				analysis.AppliedGrouping{
					GroupBy:   []analysis.DataRef{{PartitionName: "group_numbers"}},
					Precision: 1,
				},
				storage,
			)

			// Get a partition configured for a grouped sum aggregation using the grouped storage
			groupedSumPartition := analysis.NewGroupedAggregationPartition(
				general.SumAggregation,
				analysis.AppliedAggregation{
					Name:         "grouped_sum",
					Data:         analysis.DataRef{PartitionName: "gamma_compound_poisson"},
					DefaultValue: 0,
				},
				groupedStorage,
			)

			// Run and add the grouped sum aggregation partition to the original storage
			storage = analysis.AddPartitionsToStateTimeStorage(
				storage,
				[]*simulator.PartitionConfig{groupedSumPartition},
				map[string]int{"gamma_compound_poisson": 1, "group_numbers": 1},
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{Plotting: &analysis.DataPlotting{IsTime: true}}

			// Reference the plotting data for the y-axis
			yRefs := []analysis.DataRef{{PartitionName: "grouped_sum"}}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)

		},
	)
}
