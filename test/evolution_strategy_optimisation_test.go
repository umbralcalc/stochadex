package main

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
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
						Name:    "es_covariance",
						Default: []float64{4.0, 0.0, 0.0, 4.0},
						// A slow covariance learning rate lets the mean reach the
						// optimum before the search width contracts; a fast rate
						// collapses the covariance and freezes the mean short of it.
						LearningRate: 0.1,
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
						// The objective is static (the sample is fixed across the
						// window), so any discount only rescales a constant reward and
						// leaves the ranking unchanged — zero keeps it simplest.
						DiscountFactor: 0.0,
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
					MaxNumberOfSteps: 1000,
				},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)

			// The mean update should converge on the reward's target: with
			// the divergence and init-placeholder bugs fixed and the covariance
			// learning rate slow enough to avoid premature contraction, the
			// search settles on the optimum rather than stalling short of it.
			meanHistory := storage.GetValues("es_mean")
			finalMean := meanHistory[len(meanHistory)-1]
			target := []float64{3.0, -2.0}
			if !floats.EqualApprox(finalMean, target, 1e-2) {
				t.Errorf(
					"es_mean did not converge on the optimum: got %v, want %v",
					finalMean, target,
				)
			}
		},
	)
}
