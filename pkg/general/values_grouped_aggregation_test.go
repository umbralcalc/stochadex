package general

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
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

	t.Run("error panics at an undeclared latest group, naming it", func(t *testing.T) {
		settings := groupedOnlyZeroAndOne(t)
		aggregation := iterations(UndeclaredGroupsError)[2]
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
		defer func() {
			if message, _ := recover().(string); !strings.Contains(message,
				"values_grouped_aggregation: group (2.00) is not declared") {
				t.Errorf("expected a panic naming group 2.00, got %q", message)
			}
		}()
		aggregation.Iterate(&params, 2, histories, timesteps)
	})
}
