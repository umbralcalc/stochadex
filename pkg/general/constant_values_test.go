package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestConstantValues(t *testing.T) {
	t.Run(
		"test that the constant values iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./constant_values_settings.yaml")
			iteration := &ConstantValuesIteration{}
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
