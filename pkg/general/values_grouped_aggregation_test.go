package general

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

func TestValuesGroupedAggregationIteration(t *testing.T) {
	t.Run(
		"test that the values grouped aggregation iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_grouped_aggregation_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationOne.Configure(0, settings)
			iterationTwo := &ConstantValuesIteration{}
			iterationTwo.Configure(1, settings)
			iterationThree := &ValuesGroupedAggregationIteration{
				Aggregation: CountAggregation,
				Kernel:      &kernels.InstantaneousIntegrationKernel{},
			}
			iterationThree.Configure(2, settings)
			iterationFour := &ValuesGroupedAggregationIteration{
				Aggregation: MeanAggregation,
				Kernel:      &kernels.InstantaneousIntegrationKernel{},
			}
			iterationFour.Configure(3, settings)
			iterations := []simulator.Iteration{
				iterationOne,
				iterationTwo,
				iterationThree,
				iterationFour,
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
	t.Run(
		"test that the values grouped aggregation iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_grouped_aggregation_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationTwo := &ConstantValuesIteration{}
			iterationThree := &ValuesGroupedAggregationIteration{
				Aggregation: CountAggregation,
				Kernel:      &kernels.InstantaneousIntegrationKernel{},
			}
			iterationFour := &ValuesGroupedAggregationIteration{
				Aggregation: MeanAggregation,
				Kernel:      &kernels.InstantaneousIntegrationKernel{},
			}
			iterations := []simulator.Iteration{
				iterationOne,
				iterationTwo,
				iterationThree,
				iterationFour,
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

// groupedOnlyZeroAndOne configures partition_2 of the colocated settings, whose
// groupings hold groups 0, 1 and 2, to accept only groups 0 and 1.
func groupedOnlyZeroAndOne(t *testing.T) *simulator.Settings {
	t.Helper()
	settings := simulator.LoadSettingsFromYaml("./values_grouped_aggregation_settings.yaml")
	settings.Iterations[2].Params.Set("accepted_value_group_tupindex_0", []float64{0, 1})
	return settings
}

func TestUndeclaredGroupsPolicies(t *testing.T) {
	iterations := func(policy UndeclaredGroups) []simulator.Iteration {
		return []simulator.Iteration{
			&ConstantValuesIteration{},
			&ConstantValuesIteration{},
			&ValuesGroupedAggregationIteration{Aggregation: SumAggregation,
				Kernel: &kernels.InstantaneousIntegrationKernel{}, Undeclared: policy},
			&ValuesGroupedAggregationIteration{Aggregation: MeanAggregation,
				Kernel: &kernels.InstantaneousIntegrationKernel{}},
		}
	}
	implementations := func(policy UndeclaredGroups) *simulator.Implementations {
		return &simulator.Implementations{
			Iterations:           iterations(policy),
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 20},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
	}

	t.Run("other passes the harnesses", func(t *testing.T) {
		if err := simulator.RunWithHarnesses(groupedOnlyZeroAndOne(t),
			implementations(UndeclaredGroupsOther)); err != nil {
			t.Errorf("test harness failed: %v", err)
		}
	})

	// iterateOnce configures partition_2 under policy and iterates it once on
	// the colocated settings' rows: groups [0 1 2 2 2 0 1 0 1 2] and values
	// [5 2 48 11 621 1 1 2 8 245]. The instantaneous kernel weighs only the
	// latest rows, so each output is a plain sum over them.
	iterateOnce := func(t *testing.T, policy UndeclaredGroups, settings *simulator.Settings) []float64 {
		t.Helper()
		aggregation := iterations(policy)[2]
		aggregation.Configure(2, settings)
		histories := make([]*simulator.StateHistory, len(settings.Iterations))
		for index, partition := range settings.Iterations {
			histories[index] = simulator.NewStateHistoryFromWindow(
				partition.InitStateValues, partition.StateWidth, 1)
		}
		params := settings.Iterations[2].Params
		params.Set("latest_groupings_tupindex_0", settings.Iterations[0].InitStateValues)
		params.Set("latest_states", settings.Iterations[1].InitStateValues)
		timesteps := &simulator.CumulativeTimestepsHistory{
			Values: mat.NewVecDense(1, []float64{0}), NextIncrement: 1, StateHistoryDepth: 1}
		return aggregation.Iterate(&params, 2, histories, timesteps)
	}

	t.Run("drop sums the declared groups and leaves the rest out", func(t *testing.T) {
		got := iterateOnce(t, UndeclaredGroupsDrop, groupedOnlyZeroAndOne(t))
		// Group 0: 5+1+2; group 1: 2+1+8; group 2 is undeclared.
		if want := []float64{8, 11, 0}; !floats.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("other sums the undeclared groups into one more output", func(t *testing.T) {
		got := iterateOnce(t, UndeclaredGroupsOther, groupedOnlyZeroAndOne(t))
		// Group 2's values, 48+11+621+245, are the other output.
		if want := []float64{8, 11, 925}; !floats.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("other gathers every undeclared group, not just one", func(t *testing.T) {
		settings := groupedOnlyZeroAndOne(t)
		settings.Iterations[2].Params.Set("accepted_value_group_tupindex_0", []float64{0})
		settings.Iterations[2].StateWidth = 2
		// Groups 1 (2+1+8) and 2 (48+11+621+245) together.
		if got, want := iterateOnce(t, UndeclaredGroupsOther, settings), []float64{8, 936}; !floats.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("other gathers in group-key order, whatever the map order", func(t *testing.T) {
		// Groups 0, 1 and 2 hold 1e16, 1 and -1e16, and only group 5 is
		// declared. Their float64 sum depends on the order they are added in
		// (1e16 + 1 rounds to 1e16), so if gathering followed Go's randomised
		// map order, repeated runs would disagree. Which value the fixed order
		// gives depends on how floats.Dot accumulates on the platform, so the
		// claim is only that every run gives the same one.
		var first float64
		for attempt := 0; attempt < 100; attempt++ {
			settings := groupedOnlyZeroAndOne(t)
			settings.Iterations[0].InitStateValues = []float64{0, 1, 2, 5, 5, 5, 5, 5, 5, 5}
			settings.Iterations[1].InitStateValues = []float64{1e16, 1, -1e16, 0, 0, 0, 0, 0, 0, 0}
			settings.Iterations[2].Params.Set("accepted_value_group_tupindex_0", []float64{5})
			settings.Iterations[2].StateWidth = 2
			got := iterateOnce(t, UndeclaredGroupsOther, settings)[1]
			if attempt == 0 {
				first = got
			} else if got != first {
				t.Fatalf("attempt %d: other is %v, but attempt 0 gave %v", attempt, got, first)
			}
		}
	})

	t.Run("other refuses a state not one wider than the declared groups", func(t *testing.T) {
		for _, width := range []int{2, 4} {
			settings := groupedOnlyZeroAndOne(t)
			settings.Iterations[2].StateWidth = width
			func() {
				defer func() {
					want := fmt.Sprintf(`values_grouped_aggregation: partition "partition_2" declares 2 `+
						`groups, so with undeclared groups gathered as "other" its state is 3 wide, not %d`, width)
					if message, _ := recover().(string); message != want {
						t.Errorf("width %d: panicked with %q, want %q", width, message, want)
					}
				}()
				iterations(UndeclaredGroupsOther)[2].Configure(2, settings)
			}()
		}
	})

	t.Run("error panics at an undeclared latest group, naming it", func(t *testing.T) {
		defer func() {
			if message, _ := recover().(string); !strings.Contains(message,
				"values_grouped_aggregation: group (2.00) is not declared") {
				t.Errorf("expected a panic naming group 2.00, got %q", message)
			}
		}()
		iterateOnce(t, UndeclaredGroupsError, groupedOnlyZeroAndOne(t))
	})

	t.Run("error passes when every latest group is declared", func(t *testing.T) {
		settings := groupedOnlyZeroAndOne(t)
		settings.Iterations[2].Params.Set("accepted_value_group_tupindex_0", []float64{0, 1, 2})
		if got, want := iterateOnce(t, UndeclaredGroupsError, settings), []float64{8, 11, 925}; !floats.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestValuesGroupedAggregationExponentialKernel(t *testing.T) {
	// The values partition outputs [t, 10t, t^2] at time t, grouped as
	// [1, 2, 1], so group 1 holds t + t^2 and group 2 holds 10t.
	iterations := func() []simulator.Iteration {
		return []simulator.Iteration{
			&ConstantValuesIteration{},
			&ValuesFunctionIteration{Function: func(
				params *simulator.Params,
				partitionIndex int,
				stateHistories []*simulator.StateHistory,
				timestepsHistory *simulator.CumulativeTimestepsHistory,
			) []float64 {
				time := timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
				return []float64{time, 10 * time, time * time}
			}},
			&ValuesGroupedAggregationIteration{
				Aggregation: SumAggregation,
				Kernel:      &kernels.ExponentialIntegrationKernel{},
			},
		}
	}
	implementations := func(store *simulator.StateTimeStorage) *simulator.Implementations {
		return &simulator.Implementations{
			Iterations:           iterations(),
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 10},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
	}

	t.Run("weights the window by the exponential kernel's timescale", func(t *testing.T) {
		settings := simulator.LoadSettingsFromYaml(
			"./values_grouped_aggregation_exponential_settings.yaml")
		store := simulator.NewStateTimeStorage()
		impl := implementations(store)
		for index, iteration := range impl.Iterations {
			iteration.Configure(index, settings)
		}
		simulator.NewPartitionCoordinator(settings, impl).Run()
		// At time T the window is the latest row and the three before it, at
		// T-d for d = 0..3, each weighted exp(-d/2) with timescale 2.
		times, rows := store.GetTimes(), store.GetValues("aggregation")
		checked := 0
		for step, time := range times {
			if time < 3 {
				continue
			}
			var group1, group2 float64
			for d := 0.0; d <= 3; d++ {
				past := time - d
				weight := math.Exp(-d / 2)
				group1 += weight * (past + past*past)
				group2 += weight * 10 * past
			}
			if want := []float64{group1, group2}; !floats.EqualApprox(rows[step], want, 1e-9) {
				t.Errorf("time %v: got %v, want %v", time, rows[step], want)
			}
			checked++
		}
		if checked == 0 {
			t.Fatal("no steps with a full window were checked")
		}
	})

	t.Run("weights the window by the exponential kernel with harnesses", func(t *testing.T) {
		settings := simulator.LoadSettingsFromYaml(
			"./values_grouped_aggregation_exponential_settings.yaml")
		if err := simulator.RunWithHarnesses(
			settings, implementations(simulator.NewStateTimeStorage())); err != nil {
			t.Errorf("test harness failed: %v", err)
		}
	})
}
