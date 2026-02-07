package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesSortedCollectionCovariance(t *testing.T) {
	t.Run(
		"test that the sorted collection covariance iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./values_sorted_collection_covariance_settings.yaml")
			iterationSorting := &ValuesSortingCollectionIteration{
				PushAndSort: ParamValuesPushAndSortFunction,
			}
			iterationSorting.Configure(0, settings)
			iterationMean := &ValuesSortedCollectionMeanIteration{}
			iterationMean.Configure(1, settings)
			iterationCov := &ValuesSortedCollectionCovarianceIteration{}
			iterationCov.Configure(2, settings)
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationSorting,
					iterationMean,
					iterationCov,
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
		"test that the sorted collection covariance iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./values_sorted_collection_covariance_settings.yaml")
			iterationSorting := &ValuesSortingCollectionIteration{
				PushAndSort: ParamValuesPushAndSortFunction,
			}
			iterationMean := &ValuesSortedCollectionMeanIteration{}
			iterationCov := &ValuesSortedCollectionCovarianceIteration{}
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationSorting,
					iterationMean,
					iterationCov,
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
