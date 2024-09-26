package general

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCopyValues(t *testing.T) {
	t.Run(
		"test that the copy values iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./copy_values_settings.yaml")
			iterationOne := &ConstantValuesIteration{}
			iterationOne.Configure(0, settings)
			iterationTwo := &CopyValuesIteration{}
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
