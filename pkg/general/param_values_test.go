package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestParamValues(t *testing.T) {
	t.Run(
		"test that the param values iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./param_values_settings.yaml")
			iteration := &ParamValuesIteration{}
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
