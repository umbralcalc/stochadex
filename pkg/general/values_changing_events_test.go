package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesChangingEvents(t *testing.T) {
	t.Run(
		"test that the values changing events iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_changing_events_settings.yaml")
			iterationOne := &ValuesChangingEventsIteration{
				EventIteration: &ValuesFunctionIteration{
					Function: ParamsEventFunction,
				},
				IterationByEvent: map[float64]simulator.Iteration{
					1: &ValuesFunctionIteration{
						Function: func(
							params *simulator.Params,
							partitionIndex int,
							stateHistories []*simulator.StateHistory,
							timestepsHistory *simulator.CumulativeTimestepsHistory,
						) []float64 {
							return []float64{1.0}
						},
					},
				},
			}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValuesChangingEventsIteration{
				EventIteration: &ValuesFunctionIteration{
					Function: PartitionEventFunction,
				},
				IterationByEvent: map[float64]simulator.Iteration{
					1: &ValuesFunctionIteration{
						Function: func(
							params *simulator.Params,
							partitionIndex int,
							stateHistories []*simulator.StateHistory,
							timestepsHistory *simulator.CumulativeTimestepsHistory,
						) []float64 {
							return []float64{321.0}
						},
					},
				},
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
}
