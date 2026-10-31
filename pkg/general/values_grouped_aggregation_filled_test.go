package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// groupedRows are (group, value) rows replayed one per step. Group 0 is a real
// group, so history rows a run has not filled, which read as zeros, would
// count towards it if they were mistaken for data.
var groupedRows = [][]float64{{0, 7}, {1, 0}, {0, 1}, {1, 3}, {0, 1}, {1, 2}}

// groupedCountGenerator replays groupedRows and counts its values per group
// over a window as long as the replay.
func groupedCountGenerator(store *simulator.StateTimeStorage) *simulator.ConfigGenerator {
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: len(groupedRows) - 1,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	})
	generator.SetPartition(&simulator.PartitionConfig{
		Name:              "rows",
		Iteration:         &FromStorageIteration{Data: groupedRows},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   groupedRows[0],
		StateHistoryDepth: len(groupedRows) - 1,
	})
	generator.SetPartition(&simulator.PartitionConfig{
		Name: "counts",
		Iteration: &ValuesGroupedAggregationIteration{
			Aggregation: CountAggregation,
			Kernel:      &kernels.ConstantIntegrationKernel{},
		},
		Params: simulator.NewParams(map[string][]float64{
			"accepted_value_group_tupindex_0":   {0, 1},
			"float_precision":                   {0},
			"state_value_indices":               {1},
			"grouping_value_indices_tupindex_0": {0},
			"default_values":                    {0, 0},
		}),
		ParamsAsPartitions: map[string][]string{
			"state_partition":               {"rows"},
			"grouping_partition_tupindex_0": {"rows"},
		},
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"latest_states":               {Upstream: "rows"},
			"latest_groupings_tupindex_0": {Upstream: "rows", Indices: []int{0}},
		},
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
	})
	return generator
}

func TestValuesGroupedAggregationCountsOnlyFilledRows(t *testing.T) {
	t.Run("each step counts the rows so far, not the unfilled history", func(t *testing.T) {
		store := simulator.NewStateTimeStorage()
		settings, implementations := groupedCountGenerator(store).GenerateConfigs()
		simulator.NewPartitionCoordinator(settings, implementations).Run()
		counts := store.GetValues("counts")
		for step := 1; step < len(groupedRows); step++ {
			want := []float64{0, 0}
			for _, row := range groupedRows[:step+1] {
				want[int(row[0])]++
			}
			if !floats.Equal(counts[step], want) {
				t.Errorf("after step %d: counts %v, want %v from rows 0..%d", step, counts[step], want, step)
			}
		}
	})
	t.Run("it passes the harnesses", func(t *testing.T) {
		settings, implementations := groupedCountGenerator(simulator.NewStateTimeStorage()).GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness failed: %v", err)
		}
	})
}
