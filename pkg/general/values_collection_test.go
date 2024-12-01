package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesCollection(t *testing.T) {
	t.Run(
		"test that the values collection iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_collection_settings.yaml")
			iterationOne := &ValuesCollectionIteration{
				PushFunction: ParamValuesPushFunction,
			}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValuesCollectionIteration{
				PushFunction: OtherPartitionPushFunction,
			}
			iterationTwo.Configure(1, settings)
			iterationThree := &ValuesCollectionIteration{
				PopIndexFunction: NextNonEmptyPopIndexFunction,
			}
			iterationThree.Configure(2, settings)
			iterationFour := &ValuesCollectionIteration{
				PushFunction:     ParamValuesPushFunction,
				PopIndexFunction: NextNonEmptyPopIndexFunction,
			}
			iterationFour.Configure(3, settings)
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					iterationOne,
					iterationTwo,
					iterationThree,
					iterationFour,
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
}
