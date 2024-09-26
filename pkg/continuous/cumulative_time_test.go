package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCumulativeTime(t *testing.T) {
	t.Run(
		"test that the cumulative time iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("cumulative_time_settings.yaml")
			iteration := &CumulativeTimeIteration{}
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
