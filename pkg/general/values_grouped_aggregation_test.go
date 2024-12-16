package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
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
