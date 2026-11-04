package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
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
	t.Run(
		"test that the covariance blends in the full weighted covariance matrix",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./values_sorted_collection_covariance_settings.yaml")
			iterations := []simulator.Iteration{
				&ValuesSortingCollectionIteration{
					PushAndSort: ParamValuesPushAndSortFunction,
				},
				&ValuesSortedCollectionMeanIteration{},
				&ValuesSortedCollectionCovarianceIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			store := simulator.NewStateTimeStorage()
			simulator.NewPartitionCoordinator(settings, &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 10,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}).Run()
			collections := store.GetValues("sorting_collection")
			got := store.GetValues("covariance_update")
			params := settings.Iterations[2].Params
			weights := params.Get("weights")
			rate := params.GetIndex("learning_rate", 0)
			// Recompute each step from the collection it reads within the step:
			// the weighted covariance of the leading entries' values (each
			// entry is a sort key then two values) around their weighted mean,
			// blended into the previous matrix entry by entry.
			want := append([]float64(nil), settings.Iterations[2].InitStateValues...)
			for step := 1; step < len(got); step++ {
				collection := collections[step]
				mean := make([]float64, 2)
				for e, w := range weights {
					for i := range 2 {
						mean[i] += w * collection[e*3+1+i]
					}
				}
				next := make([]float64, 4)
				for i := range 2 {
					for j := range 2 {
						cov := 0.0
						for e, w := range weights {
							cov += w * (collection[e*3+1+i] - mean[i]) *
								(collection[e*3+1+j] - mean[j])
						}
						next[i*2+j] = (1-rate)*want[i*2+j] + rate*cov
					}
				}
				want = next
				if !floats.EqualApprox(got[step], want, 1e-12) {
					t.Fatalf("step %d: covariance %v, want %v", step, got[step], want)
				}
			}
			if final := got[len(got)-1]; final[1] == 0 {
				t.Errorf("final covariance %v should have off-diagonal entries", final)
			}
		},
	)
}
