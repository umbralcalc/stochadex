package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestOrnsteinUhlenbeckProcess(t *testing.T) {
	t.Run(
		"test that the Ornstein-Uhlenbeck process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./ornstein_uhlenbeck_settings.yaml")
			partitions := make([]simulator.Partition, 0)
			for partitionIndex := range settings.StateWidths {
				iteration := &OrnsteinUhlenbeckIteration{}
				iteration.Configure(partitionIndex, settings)
				partitions = append(partitions, simulator.Partition{Iteration: iteration})
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
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
