package macros

import (
	"maps"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// EvolutionStrategyRewardWiring is an evolution strategy's reward as data: the
// per-step reward partition's params, wiring and state width, the params it reads
// from outside the simulation, and the discount its accumulation applies.
type EvolutionStrategyRewardWiring struct {
	Params             map[string][]float64
	ParamsAsPartitions map[string][]string
	ParamsFromUpstream map[string]simulator.NamedUpstreamConfig
	Width              int
	OutsideUpstreams   map[string]simulator.NamedUpstreamConfig
	DiscountFactor     float64
}

// EvolutionStrategyLayout is the data an evolution strategy's partitions are wired
// with: everything about them but their iterations. Simulation is the host that runs
// the window and the discounted reward, named SimulationName; Reward is that reward,
// named "discounted_reward" inside it. The constructors here and a config's expansion
// both read it.
type EvolutionStrategyLayout struct {
	SimulationName                                         string
	Simulation, Reward, Sampler, Sorting, Mean, Covariance InnerLayout
}

// NewEvolutionStrategyLayout is the layout of an evolution strategy whose window runs
// partitions and replays data before its reward. The reward accumulates from the
// sorting sentinel, so the never-sampled first point ranks last rather than at the top.
func NewEvolutionStrategyLayout(
	sampler EvolutionStrategySampler,
	sorting EvolutionStrategySorting,
	mean EvolutionStrategyMean,
	covariance EvolutionStrategyCovariance,
	window []WindowPartitionRef,
	replays []WindowReplay,
	reward EvolutionStrategyRewardWiring,
) EvolutionStrategyLayout {
	simulation := InnerLayout{
		Params:             map[string][]float64{"burn_in_steps": {0}},
		ParamsAsPartitions: map[string][]string{},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{},
		InitStateValues:    []float64{},
	}
	outside := func(partition string, upstreams map[string]simulator.NamedUpstreamConfig) {
		for param, upstream := range upstreams {
			simulation.ParamsFromUpstream[partition+"/"+param] = upstream
		}
	}
	for _, partition := range window {
		simulation.InitStateValues = append(simulation.InitStateValues, partition.InitState...)
		outside(partition.Name, partition.OutsideUpstreams)
	}
	for _, replay := range replays {
		simulation.InitStateValues = append(simulation.InitStateValues, replay.InitState...)
		simulation.ParamsAsPartitions[replay.Name+"/update_from_partition_history"] = []string{replay.Name}
		simulation.ParamsAsPartitions[replay.Name+"/initial_state_from_partition_history"] = []string{replay.Name}
	}
	rewardParams := cloneOrEmpty(reward.Params)
	rewardParams["discount_factor"] = []float64{reward.DiscountFactor}
	rewardInit := repeatedValue(sorting.EmptyValue, reward.Width)
	sortBy := len(simulation.InitStateValues)
	simulation.InitStateValues = append(simulation.InitStateValues, rewardInit...)
	outside("discounted_reward", reward.OutsideUpstreams)

	dimension := len(sampler.Default)
	sampleIndices := make([]float64, dimension)
	for i := range sampleIndices {
		sampleIndices[i] = float64(i)
	}
	simulationName := sampler.Name + "_simulation"
	update := func(learningRate float64, initState []float64) InnerLayout {
		return InnerLayout{
			Params: map[string][]float64{
				"weights":            mean.Weights,
				"learning_rate":      {learningRate},
				"values_state_width": {float64(dimension)},
				"empty_value":        {sorting.EmptyValue},
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"sorted_collection": {Upstream: sorting.Name}},
			InitStateValues: initState,
		}
	}
	return EvolutionStrategyLayout{
		SimulationName: simulationName,
		Simulation:     simulation,
		Reward: InnerLayout{
			Params:             rewardParams,
			ParamsAsPartitions: maps.Clone(reward.ParamsAsPartitions),
			ParamsFromUpstream: maps.Clone(reward.ParamsFromUpstream),
			InitStateValues:    rewardInit,
		},
		Sampler: InnerLayout{
			Params: map[string][]float64{"default_covariance": covariance.Default},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"mean":              {Upstream: mean.Name},
				"covariance_matrix": {Upstream: covariance.Name},
			},
			InitStateValues: sampler.Default,
		},
		Sorting: InnerLayout{
			Params: map[string][]float64{
				"value_indices":       sampleIndices,
				"value_index_sort_by": {float64(sortBy)},
				"empty_value":         {sorting.EmptyValue},
				"values_state_width":  {float64(dimension)},
			},
			ParamsAsPartitions: map[string][]string{
				"other_partition":         {sampler.Name},
				"other_partition_sort_by": {simulationName},
			},
			InitStateValues: repeatedValue(sorting.EmptyValue, sorting.CollectionSize*(dimension+1)),
		},
		Mean:       update(mean.LearningRate, mean.Default),
		Covariance: update(covariance.LearningRate, covariance.Default),
	}
}
