package analysis

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
	sampleDim := len(applied.Sampler.Default)
	partitions := make([]*simulator.PartitionConfig, 0)

	// Build the embedded simulation containing user partitions
	// and a discounted reward accumulation
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:  &simulator.NilOutputCondition{},
		OutputFunction:   &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.Window.Depth,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:    0.0,
	})
	simInitStateValues := make([]float64, 0)
	simParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	simParamsAsPartitions := make(map[string][]string)
	if applied.Window.Partitions != nil {
		for _, partition := range applied.Window.Partitions {
			generator.SetPartition(partition.Partition)
			simInitStateValues = append(
				simInitStateValues,
				partition.Partition.InitStateValues...,
			)
			if partition.OutsideUpstreams == nil {
				continue
			}
			for paramsName, upstream := range partition.OutsideUpstreams {
				simParamsFromUpstream[partition.Partition.Name+
					"/"+paramsName] = upstream
			}
		}
	}
	if applied.Window.Data != nil {
		for _, ref := range applied.Window.Data {
			if ref.ValueIndices != nil {
				panic("value indices are not supported in window data")
			}
			initStateValues := ref.GetTimeIndexFromStorage(storage, 0)
			generator.SetPartition(&simulator.PartitionConfig{
				Name:              ref.PartitionName,
				Iteration:         &general.FromHistoryIteration{},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   initStateValues,
				StateHistoryDepth: 1,
				Seed:              0,
			})
			simInitStateValues = append(simInitStateValues, initStateValues...)
			simParamsAsPartitions[ref.PartitionName+
				"/update_from_partition_history"] = []string{ref.PartitionName}
			simParamsAsPartitions[ref.PartitionName+
				"/initial_state_from_partition_history"] = []string{ref.PartitionName}
		}
	}

	// Add the reward partition wrapped with discounted cumulative
	rewardPartition := applied.Reward.Partition.Partition
	rewardParams := simulator.NewParams(make(map[string][]float64))
	for k, v := range rewardPartition.Params.Map {
		rewardParams.Set(k, v)
	}
	rewardParams.Set(
		"discount_factor",
		[]float64{applied.Reward.DiscountFactor},
	)
	rewardParamsAsPartitions := make(map[string][]string)
	if rewardPartition.ParamsAsPartitions != nil {
		for k, v := range rewardPartition.ParamsAsPartitions {
			rewardParamsAsPartitions[k] = v
		}
	}
	rewardParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	if rewardPartition.ParamsFromUpstream != nil {
		for k, v := range rewardPartition.ParamsFromUpstream {
			rewardParamsFromUpstream[k] = v
		}
	}
	rewardInitState := make(
		[]float64, len(rewardPartition.InitStateValues))
	copy(rewardInitState, rewardPartition.InitStateValues)
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "discounted_reward",
		Iteration: &general.DiscountedCumulativeIteration{
			Iteration: rewardPartition.Iteration,
		},
		Params:             rewardParams,
		ParamsAsPartitions: rewardParamsAsPartitions,
		ParamsFromUpstream: rewardParamsFromUpstream,
		InitStateValues:    rewardInitState,
		StateHistoryDepth:  1,
		Seed:               0,
	})
	if applied.Reward.Partition.OutsideUpstreams != nil {
		for paramsName, upstream := range applied.Reward.Partition.OutsideUpstreams {
			simParamsFromUpstream["discounted_reward/"+paramsName] = upstream
		}
	}
	simInitStateValues = append(simInitStateValues, rewardInitState...)

	// Compute the sort-by index: offset of discounted_reward in
	// the concatenated embedded sim output
	sortByIndex := len(simInitStateValues) - len(rewardInitState)

	simName := applied.Sampler.Name + "_simulation"
	simParams := simulator.NewParams(map[string][]float64{
		"burn_in_steps": {0},
	})
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: simName,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			generator.GenerateConfigs(),
		),
		Params:             simParams,
		ParamsAsPartitions: simParamsAsPartitions,
		ParamsFromUpstream: simParamsFromUpstream,
		InitStateValues:    simInitStateValues,
		StateHistoryDepth:  1,
		Seed:               0,
	})

	// Sampler: draws from N(mean, covariance) where mean and covariance
	// come from the update partitions
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.Sampler.Name,
		Iteration: &inference.DataGenerationIteration{
			Likelihood: &inference.NormalLikelihoodDistribution{},
		},
		Params: simulator.NewParams(map[string][]float64{
			"default_covariance": applied.Covariance.Default,
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"mean":              {Upstream: applied.Mean.Name},
			"covariance_matrix": {Upstream: applied.Covariance.Name},
		},
		InitStateValues:   applied.Sampler.Default,
		StateHistoryDepth: 1,
		Seed:              applied.Seed,
	})

	// Sorting collection: ranks samples by discounted return
	entryWidth := sampleDim + 1
	collectionStateWidth := applied.Sorting.CollectionSize * entryWidth
	sortInitState := make([]float64, collectionStateWidth)
	for i := range sortInitState {
		sortInitState[i] = applied.Sorting.EmptyValue
	}
	sampleValueIndices := make([]float64, sampleDim)
	for i := range sampleDim {
		sampleValueIndices[i] = float64(i)
	}
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.Sorting.Name,
		Iteration: &general.ValuesSortingCollectionIteration{
			PushAndSort: general.OtherPartitionsPushAndSortFunction,
		},
		Params: simulator.NewParams(map[string][]float64{
			"value_indices":      sampleValueIndices,
			"value_index_sort_by": {float64(sortByIndex)},
			"empty_value":        {applied.Sorting.EmptyValue},
			"values_state_width": {float64(sampleDim)},
		}),
		ParamsAsPartitions: map[string][]string{
			"other_partition":         {applied.Sampler.Name},
			"other_partition_sort_by": {simName},
		},
		InitStateValues:   sortInitState,
		StateHistoryDepth: 2,
		Seed:              0,
	})

	// Mean update: weighted mean of top-ranked samples blended with
	// previous mean
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.Mean.Name,
		Iteration: &general.ValuesSortedCollectionMeanIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"weights":           applied.Mean.Weights,
			"learning_rate":     {applied.Mean.LearningRate},
			"values_state_width": {float64(sampleDim)},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"sorted_collection": {Upstream: applied.Sorting.Name},
		},
		InitStateValues:   applied.Mean.Default,
		StateHistoryDepth: 1,
		Seed:              0,
	})

	// Covariance update: weighted covariance around the updated mean
	// blended with previous covariance
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.Covariance.Name,
		Iteration: &general.ValuesSortedCollectionCovarianceIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"weights":           applied.Mean.Weights,
			"learning_rate":     {applied.Covariance.LearningRate},
			"values_state_width": {float64(sampleDim)},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"sorted_collection": {Upstream: applied.Sorting.Name},
			"mean":              {Upstream: applied.Mean.Name},
		},
		InitStateValues:   applied.Covariance.Default,
		StateHistoryDepth: 1,
		Seed:              0,
	})

	return partitions
}
