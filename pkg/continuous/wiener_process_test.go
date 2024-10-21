package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestWienerProcess(t *testing.T) {
	t.Run(
		"test that the Wiener process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./wiener_process_settings.yaml")
			partitions := make([]simulator.Partition, 0)
			for partitionIndex := range settings.StateWidths {
				iteration := &WienerProcessIteration{}
				iteration.Configure(partitionIndex, settings)
				partitions = append(partitions, simulator.Partition{Iteration: iteration})
			}
			store := make(map[string][][]float64)
			implementations := &simulator.Implementations{
				Partitions:      partitions,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
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
