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
			iterationOne := &ValuesFunctionIteration{
				Function: func(
					params *simulator.Params,
					partitionIndex int,
					stateHistories []*simulator.StateHistory,
					timestepsHistory *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{1345.0}
				},
			}
			iterationOne.Configure(0, settings)
			iterationTwo := &ValuesFunctionIteration{
				Function: NewTransformReduceFunction(ParamsTransform, SumReduce),
			}
			iterationTwo.Configure(1, settings)
			implementations := &simulator.Implementations{
				Iterations:      []simulator.Iteration{iterationOne, iterationTwo},
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
