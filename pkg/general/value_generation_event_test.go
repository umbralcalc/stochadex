package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValueGenerationEvent(t *testing.T) {
	t.Run(
		"test that the value generation event iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./value_generation_event_settings.yaml")
			iterationOne := &ValueGenerationEventIteration{
				ValueIteration: &ValueFunctionIteration{
					Function: func(
						params simulator.Params,
						partitionIndex int,
						stateHistories []*simulator.StateHistory,
						timestepsHistory *simulator.CumulativeTimestepsHistory,
					) []float64 {
						return []float64{1.0}
					},
				},
			}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValueGenerationEventIteration{
				ValueIteration: &ValueFunctionIteration{
					Function: func(
						params simulator.Params,
						partitionIndex int,
						stateHistories []*simulator.StateHistory,
						timestepsHistory *simulator.CumulativeTimestepsHistory,
					) []float64 {
						return []float64{321.0}
					},
				},
			}
			iterationTwo.Configure(1, settings)
			partitions := []simulator.Partition{{Iteration: iterationOne}, {Iteration: iterationTwo}}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
