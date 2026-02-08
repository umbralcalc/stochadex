package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// NegativeSquaredDistanceIteration computes a scalar reward equal to the
// negative squared Euclidean distance between sampled parameter values and
// a fixed target vector. It can be used as a per-step reward iteration
// inside an embedded simulation for evolution strategies optimisation.
type NegativeSquaredDistanceIteration struct {
}

func (n *NegativeSquaredDistanceIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (n *NegativeSquaredDistanceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	target := params.Get("target")
	sample := params.Get("sample_values")
	reward := 0.0
	for i := range target {
		diff := sample[i] - target[i]
		reward -= diff * diff
	}
	return []float64{reward}
}

func TestEvolutionStrategyOptimisation(t *testing.T) {
	t.Run(
		"integration test: evolution strategy optimisation",
		func(t *testing.T) {
			// Configure the evolution strategies optimisation partitions
			// to find 2D parameters which maximise a reward (negative
			// squared distance from target)
			partitions := analysis.NewEvolutionStrategyOptimisationPartitions(
				analysis.AppliedEvolutionStrategyOptimisation{
					Sampler: analysis.EvolutionStrategySampler{
						Name:    "es_sampler",
						Default: []float64{0.0, 0.0},
					},
					Sorting: analysis.EvolutionStrategySorting{
						Name:           "es_sorting",
						CollectionSize: 10,
						EmptyValue:     -9999.0,
					},
					Mean: analysis.EvolutionStrategyMean{
						Name:         "es_mean",
						Default:      []float64{0.0, 0.0},
						Weights:      []float64{0.5, 0.3, 0.2},
						LearningRate: 0.5,
					},
					Covariance: analysis.EvolutionStrategyCovariance{
						Name:         "es_covariance",
						Default:      []float64{4.0, 0.0, 0.0, 4.0},
						LearningRate: 0.3,
					},
					Reward: analysis.EvolutionStrategyReward{
						Partition: analysis.WindowedPartition{
							Partition: &simulator.PartitionConfig{
								Name:      "reward",
								Iteration: &NegativeSquaredDistanceIteration{},
								Params: simulator.NewParams(map[string][]float64{
									"target":        {3.0, -2.0},
									"sample_values": {0.0, 0.0},
								}),
								InitStateValues:   []float64{0.0},
								StateHistoryDepth: 1,
								Seed:              0,
							},
							OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
								"sample_values": {Upstream: "es_sampler"},
							},
						},
						DiscountFactor: 0.9,
					},
					Window: analysis.WindowedPartitions{
						Partitions: []analysis.WindowedPartition{{
							Partition: &simulator.PartitionConfig{
								Name:      "sim_partition",
								Iteration: &general.ConstantValuesIteration{},
								Params: simulator.NewParams(
									make(map[string][]float64)),
								InitStateValues:   []float64{0.0},
								StateHistoryDepth: 1,
								Seed:              0,
							},
						}},
						Depth: 5,
					},
					Seed: 12345,
				},
				nil,
			)

			// Run the evolution strategies optimisation as a simulation
			storage := analysis.NewStateTimeStorageFromPartitions(
				partitions,
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)

			// Reference the plotting data for the x-axis
			xRef := analysis.DataRef{
				Plotting: &analysis.DataPlotting{IsTime: true},
			}

			// Reference the sampler plotting data for the y-axis
			yRefs := []analysis.DataRef{
				{PartitionName: "es_sampler"},
			}

			// Create a scatter plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewScatterPlotFromPartition(storage, xRef, yRefs)

			// Reference the mean update plotting data for the y-axis
			yRefs = []analysis.DataRef{
				{PartitionName: "es_mean"},
			}

			// Create a line plot from partitions in a simulator.StateTimeStorage
			_ = analysis.NewLinePlotFromPartition(storage, xRef, yRefs, nil)
		},
	)
}
