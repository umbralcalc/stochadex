package macros

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// EvolutionStrategySampler defines the configuration needed to specify
// the sampling distribution in the AppliedEvolutionStrategyOptimisation.
type EvolutionStrategySampler struct {
	Name    string
	Default []float64
}

// EvolutionStrategySorting defines the configuration needed to specify
// the sorted collection in the AppliedEvolutionStrategyOptimisation.
type EvolutionStrategySorting struct {
	Name           string
	CollectionSize int
	EmptyValue     float64
}

// EvolutionStrategyMean defines the configuration needed to specify
// the mean update in the AppliedEvolutionStrategyOptimisation.
type EvolutionStrategyMean struct {
	Name         string
	Default      []float64
	Weights      []float64
	LearningRate float64
}

// EvolutionStrategyCovariance defines the configuration needed to specify
// the covariance update in the AppliedEvolutionStrategyOptimisation.
type EvolutionStrategyCovariance struct {
	Name         string
	Default      []float64
	LearningRate float64
}

// EvolutionStrategyReward defines the per-step reward iteration to be
// wrapped with discounted cumulative accumulation inside the embedded
// simulation.
type EvolutionStrategyReward struct {
	Partition      WindowedPartition
	DiscountFactor float64
}

// AppliedEvolutionStrategyOptimisation is the base configuration for an
// online evolution strategies optimisation of discounted future returns
// calculated by an embedded simulation.
type AppliedEvolutionStrategyOptimisation struct {
	Sampler    EvolutionStrategySampler
	Sorting    EvolutionStrategySorting
	Mean       EvolutionStrategyMean
	Covariance EvolutionStrategyCovariance
	Reward     EvolutionStrategyReward
	Window     WindowedPartitions
	Seed       uint64
}

// NewEvolutionStrategyOptimisationPartitions creates a set of
// PartitionConfigs for an online evolution strategies optimisation
// process. It builds:
//  1. A sampler drawing from N(mean, covariance)
//  2. An embedded simulation running user partitions with discounted reward
//  3. A sorted collection ranking samples by return
//  4. A weighted mean update from the top-ranked samples
//  5. A weighted covariance update around the mean
func NewEvolutionStrategyOptimisationPartitions(
	applied AppliedEvolutionStrategyOptimisation,
	storage *simulator.StateTimeStorage,
) []*simulator.PartitionConfig {
	partitions := make([]*simulator.PartitionConfig, 0)

	// Build the embedded simulation containing user partitions
	// and a discounted reward accumulation
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.NilOutputCondition{},
		OutputFunction:  &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Window.Depth,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	var window []WindowPartitionRef
	for _, partition := range applied.Window.Partitions {
		generator.SetPartition(partition.Partition)
		window = append(window, WindowPartitionRef{Name: partition.Partition.Name,
			InitState: partition.Partition.InitStateValues, OutsideUpstreams: partition.OutsideUpstreams})
	}
	var replays []WindowReplay
	for _, ref := range applied.Window.Data {
		if ref.ValueIndices != nil {
			panic("value indices are not supported in window data")
		}
		replay := WindowReplay{Name: ref.PartitionName, InitState: ref.GetTimeIndexFromStorage(storage, 0)}
		replays = append(replays, replay)
		generator.SetPartition(&simulator.PartitionConfig{
			Name:              ref.PartitionName,
			Iteration:         &general.FromHistoryIteration{},
			Params:            simulator.NewParams(make(map[string][]float64)),
			InitStateValues:   replay.InitState,
			StateHistoryDepth: 1,
		})
	}
	reward := applied.Reward.Partition.Partition
	layout := NewEvolutionStrategyLayout(applied.Sampler, applied.Sorting, applied.Mean,
		applied.Covariance, window, replays, EvolutionStrategyRewardWiring{
			Params:             reward.Params.Map,
			ParamsAsPartitions: reward.ParamsAsPartitions,
			ParamsFromUpstream: reward.ParamsFromUpstream,
			Width:              len(reward.InitStateValues),
			OutsideUpstreams:   applied.Reward.Partition.OutsideUpstreams,
			DiscountFactor:     applied.Reward.DiscountFactor,
		})
	generator.SetPartition(layout.Reward.partition("discounted_reward",
		&general.DiscountedCumulativeIteration{Iteration: reward.Iteration}))
	simulation := layout.Simulation.partition(layout.SimulationName,
		general.NewEmbeddedSimulationRunIteration(generator.GenerateConfigs()))
	sampler := layout.Sampler.partition(applied.Sampler.Name, &inference.DataGenerationIteration{
		Likelihood: &inference.NormalLikelihoodDistribution{AllowDefaultCovarianceFallback: true},
	})
	sampler.Seed = applied.Seed
	sorting := layout.Sorting.partition(applied.Sorting.Name, &general.ValuesSortingCollectionIteration{
		PushAndSort: general.OtherPartitionsPushAndSortFunction,
	})
	sorting.StateHistoryDepth = 2
	partitions = append(partitions, simulation, sampler, sorting,
		layout.Mean.partition(applied.Mean.Name, &general.ValuesSortedCollectionMeanIteration{}),
		layout.Covariance.partition(applied.Covariance.Name, &general.ValuesSortedCollectionCovarianceIteration{}))
	return partitions
}
