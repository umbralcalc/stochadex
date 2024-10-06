package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesCollectionPush(t *testing.T) {
	t.Run(
		"test that the values collection push iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_collection_push_settings.yaml")
			iterationOne := &ValuesCollectionPushIteration{
				PushFunction: ParamValuesPushFunction,
			}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValuesCollectionPushIteration{
				PushFunction: OtherPartitionPushFunction,
			}
			iterationTwo.Configure(1, settings)
			partitions := []simulator.Partition{{Iteration: iterationOne}, {Iteration: iterationTwo}}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
}
