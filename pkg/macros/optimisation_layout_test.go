package macros

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// evolutionStrategy is an optimisation of the negative squared distance from (3, -2),
// undiscounted, over a window holding a constant partition that starts at 7.
func evolutionStrategy(data []analysis.DataRef) AppliedEvolutionStrategyOptimisation {
	return AppliedEvolutionStrategyOptimisation{
		Sampler: EvolutionStrategySampler{Name: "s", Default: []float64{0, 0}},
		Sorting: EvolutionStrategySorting{Name: "sort", CollectionSize: 4, EmptyValue: -9999},
		Mean: EvolutionStrategyMean{Name: "m", Default: []float64{0, 0}, Weights: []float64{0.6, 0.4},
			LearningRate: 0.5},
		Covariance: EvolutionStrategyCovariance{Name: "c", Default: []float64{4, 0, 0, 4}, LearningRate: 0.2},
		Reward: EvolutionStrategyReward{
			Partition: WindowedPartition{
				Partition: &simulator.PartitionConfig{
					Name: "reward",
					Iteration: &general.ExpressionIteration{
						Fields: []general.ExpressionField{{Name: "r"}},
						Outputs: []string{"-((sample_values[0]-3)*(sample_values[0]-3) + " +
							"(sample_values[1]+2)*(sample_values[1]+2))"},
					},
					Params:            simulator.NewParams(map[string][]float64{"sample_values": {0, 0}}),
					InitStateValues:   []float64{0},
					StateHistoryDepth: 1,
				},
				OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{"sample_values": {Upstream: "s"}},
			},
		},
		Window: WindowedPartitions{
			Partitions: []WindowedPartition{{Partition: &simulator.PartitionConfig{
				Name:              "w",
				Iteration:         &general.ConstantValuesIteration{},
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   []float64{7},
				StateHistoryDepth: 1,
			}}},
			Data:  data,
			Depth: 3,
		},
		Seed: 5,
	}
}

func TestEvolutionStrategyLayout(t *testing.T) {
	t.Run("each step scores this step's sample over the window", func(t *testing.T) {
		storage := analysis.NewStateTimeStorageFromPartitions(
			NewEvolutionStrategyOptimisationPartitions(evolutionStrategy(nil), nil),
			&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 6},
			&simulator.ConstantTimestepFunction{Stepsize: 1.0}, 0.0)
		samples, simulation := storage.GetValues("s"), storage.GetValues("s_simulation")
		if !reflect.DeepEqual(simulation[0], []float64{7, -9999}) {
			t.Errorf("the first row is %v, want the window's state and the sorting sentinel", simulation[0])
		}
		for step := 1; step < len(simulation); step++ {
			x, y := samples[step][0], samples[step][1]
			want := []float64{7, -((x-3)*(x-3) + (y+2)*(y+2))}
			if !floats.EqualApprox(simulation[step], want, 1e-12) {
				t.Errorf("step %d: %v, want %v", step, simulation[step], want)
			}
		}
	})

	storage := simulator.NewStateTimeStorage()
	storage.SetTimes([]float64{0, 1})
	storage.SetValues("obs", [][]float64{{2, 3}, {4, 5}})

	t.Run("a window replaying data lays it out after its partitions", func(t *testing.T) {
		host := NewEvolutionStrategyOptimisationPartitions(
			evolutionStrategy([]analysis.DataRef{{PartitionName: "obs"}}), storage)[0]
		if !reflect.DeepEqual(host.InitStateValues, []float64{7, 2, 3, -9999}) ||
			!reflect.DeepEqual(host.ParamsAsPartitions, map[string][]string{
				"obs/update_from_partition_history":        {"obs"},
				"obs/initial_state_from_partition_history": {"obs"}}) {
			t.Errorf("got %v and %v", host.InitStateValues, host.ParamsAsPartitions)
		}
		sorting := NewEvolutionStrategyOptimisationPartitions(
			evolutionStrategy([]analysis.DataRef{{PartitionName: "obs"}}), storage)[2]
		if got := sorting.Params.Get("value_index_sort_by"); !reflect.DeepEqual(got, []float64{3}) {
			t.Errorf("the reward is sorted by element %v, want 3", got)
		}
	})

	t.Run("a window replaying chosen values of its data panics", func(t *testing.T) {
		defer func() {
			if got := fmt.Sprint(recover()); !strings.Contains(got, "value indices are not supported") {
				t.Errorf("got %q", got)
			}
		}()
		NewEvolutionStrategyOptimisationPartitions(evolutionStrategy(
			[]analysis.DataRef{{PartitionName: "obs", ValueIndices: []int{0}}}), storage)
	})
}
