package analysis

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func TestEvolutionStrategyOptimisation(t *testing.T) {
	t.Run(
		"test that the evolution strategy optimisation works",
		func(t *testing.T) {
			partitions := NewEvolutionStrategyOptimisationPartitions(
				AppliedEvolutionStrategyOptimisation{
					Sampler: EvolutionStrategySampler{
						Name:    "test_sampler",
						Default: []float64{0.0, 0.0},
					},
					Sorting: EvolutionStrategySorting{
						Name:           "test_sorting",
						CollectionSize: 5,
						EmptyValue:     -9999.0,
					},
					Mean: EvolutionStrategyMean{
						Name:         "test_mean",
						Default:      []float64{0.0, 0.0},
						Weights:      []float64{0.6, 0.4},
						LearningRate: 0.5,
					},
					Covariance: EvolutionStrategyCovariance{
						Name:         "test_covariance",
						Default:      []float64{1.0, 0.0, 0.0, 1.0},
						LearningRate: 0.5,
					},
					Reward: EvolutionStrategyReward{
						Partition: WindowedPartition{
							Partition: &simulator.PartitionConfig{
								Name:      "reward",
								Iteration: &general.ConstantValuesIteration{},
								Params: simulator.NewParams(
									make(map[string][]float64)),
								InitStateValues:   []float64{1.0},
								StateHistoryDepth: 1,
								Seed:              0,
							},
						},
						DiscountFactor: 0.9,
					},
					Window: WindowedPartitions{
						Partitions: []WindowedPartition{{
							Partition: &simulator.PartitionConfig{
								Name:      "sim_partition",
								Iteration: &general.ConstantValuesIteration{},
								Params: simulator.NewParams(
									make(map[string][]float64)),
								InitStateValues:   []float64{1.0},
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
			storage := NewStateTimeStorageFromPartitions(
				partitions,
				&simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 20,
				},
				&simulator.ConstantTimestepFunction{Stepsize: 1.0},
				0.0,
			)
			for _, name := range storage.GetNames() {
				for _, values := range storage.GetValues(name) {
					if floats.HasNaN(values) {
						t.Error("partition " + name + " values have NaN")
					}
				}
			}
		},
	)
}
