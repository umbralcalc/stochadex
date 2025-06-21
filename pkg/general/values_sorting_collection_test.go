package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesSortingCollection(t *testing.T) {
	t.Run(
		"test that the values collection iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_sorting_collection_settings.yaml")
			iterationOne := &ValuesSortingCollectionIteration{
				PushAndSort: ParamValuesPushAndSortFunction,
			}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValuesSortingCollectionIteration{
				PushAndSort: OtherPartitionsPushAndSortFunction,
			}
			iterationTwo.Configure(1, settings)
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationOne,
					iterationTwo,
				},
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 10,
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
		"test that the values collection iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_sorting_collection_settings.yaml")
			iterationOne := &ValuesSortingCollectionIteration{
				PushAndSort: ParamValuesPushAndSortFunction,
			}
			iterationTwo := &ValuesSortingCollectionIteration{
				PushAndSort: OtherPartitionsPushAndSortFunction,
			}
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationOne,
					iterationTwo,
				},
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 10,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
