package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// evolution_strategy_optimisation is a LIVE macro: unlike the against-storage
// analysis macros, NewEvolutionStrategyOptimisationPartitions takes no data and
// its partitions form a self-contained optimisation loop that runs as a fresh
// simulation. So its spec implements liveMacroSpec (resolveLive) and its
// against-storage resolve is an error.

// liveMacroSpec is implemented by macros whose partitions run as a fresh
// simulation for Steps, rather than against pre-recorded storage. The storage
// (nil when there is no data: block) is passed through for macros — like
// smc_inference — that need observed data to drive an otherwise-live run.
type liveMacroSpec interface {
	resolveLive(storage *simulator.StateTimeStorage) (
		partitions []*simulator.PartitionConfig, steps int, timestep float64, err error)
}

type evolutionStrategySpec struct {
	macroTypeField `yaml:",inline"`
	Sampler        struct {
		Name    string    `yaml:"name"`
		Default []float64 `yaml:"default"`
	} `yaml:"sampler"`
	Sorting struct {
		Name           string  `yaml:"name"`
		CollectionSize int     `yaml:"collection_size"`
		EmptyValue     float64 `yaml:"empty_value,omitempty"`
	} `yaml:"sorting"`
	Mean struct {
		Name         string    `yaml:"name"`
		Default      []float64 `yaml:"default"`
		Weights      []float64 `yaml:"weights,omitempty"`
		LearningRate float64   `yaml:"learning_rate"`
	} `yaml:"mean"`
	Covariance struct {
		Name         string    `yaml:"name"`
		Default      []float64 `yaml:"default"`
		LearningRate float64   `yaml:"learning_rate"`
	} `yaml:"covariance"`
	Reward struct {
		Partition      windowedPartitionSpec `yaml:"partition"`
		DiscountFactor float64               `yaml:"discount_factor"`
	} `yaml:"reward"`
	Window   windowedPartitionsSpec `yaml:"window"`
	Seed     uint64                 `yaml:"seed"`
	Steps    int                    `yaml:"steps"`
	Timestep float64                `yaml:"timestep,omitempty"`
}

// resolve reports that this is a live macro (it does not run against storage).
func (s *evolutionStrategySpec) resolve(
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	return nil, nil, fmt.Errorf(
		"evolution_strategy_optimisation is a live macro; it runs its own " +
			"simulation and must not be combined with against-storage macros")
}

func (s *evolutionStrategySpec) resolveLive(
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, int, float64, error) {
	rewardPartition, err := resolveWindowedPartition(s.Reward.Partition)
	if err != nil {
		return nil, 0, 0, err
	}
	window, err := s.Window.resolve()
	if err != nil {
		return nil, 0, 0, err
	}
	applied := analysis.AppliedEvolutionStrategyOptimisation{
		Sampler:    analysis.EvolutionStrategySampler{Name: s.Sampler.Name, Default: s.Sampler.Default},
		Sorting:    analysis.EvolutionStrategySorting{Name: s.Sorting.Name, CollectionSize: s.Sorting.CollectionSize, EmptyValue: s.Sorting.EmptyValue},
		Mean:       analysis.EvolutionStrategyMean{Name: s.Mean.Name, Default: s.Mean.Default, Weights: s.Mean.Weights, LearningRate: s.Mean.LearningRate},
		Covariance: analysis.EvolutionStrategyCovariance{Name: s.Covariance.Name, Default: s.Covariance.Default, LearningRate: s.Covariance.LearningRate},
		Reward:     analysis.EvolutionStrategyReward{Partition: rewardPartition, DiscountFactor: s.Reward.DiscountFactor},
		Window:     window,
		Seed:       s.Seed,
	}
	// Storage is nil: evolution strategy inspects no pre-recorded data.
	partitions := analysis.NewEvolutionStrategyOptimisationPartitions(applied, nil)
	timestep := s.Timestep
	if timestep == 0 {
		timestep = 1.0
	}
	return partitions, s.Steps, timestep, nil
}

// resolveWindowedPartition resolves a single windowed partition spec (its inner
// partition's data-spec iteration and outside upstreams).
func resolveWindowedPartition(spec windowedPartitionSpec) (analysis.WindowedPartition, error) {
	partition := spec.Partition
	partition.Init()
	if partition.IterationSpec.IsData() {
		iteration, err := ResolveIteration(partition.IterationSpec)
		if err != nil {
			return analysis.WindowedPartition{}, fmt.Errorf("reward partition %q: %w", partition.Name, err)
		}
		partition.Iteration = iteration
	}
	return analysis.WindowedPartition{Partition: &partition, OutsideUpstreams: spec.OutsideUpstreams}, nil
}
