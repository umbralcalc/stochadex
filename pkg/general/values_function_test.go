package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesFunction(t *testing.T) {
	t.Run(
		"test that the values function iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./values_function_settings.yaml")
			iteration := &ValuesFunctionIteration{
				Function: func(
					params simulator.Params,
					partitionIndex int,
					stateHistories []*simulator.StateHistory,
					timestepsHistory *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{1345.0}
				},
			}
			iteration.Configure(0, settings)
			partitions := []simulator.Partition{{Iteration: iteration}}
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
