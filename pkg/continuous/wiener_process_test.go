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
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.Iterations {
				iteration := &WienerProcessIteration{}
				iteration.Configure(partitionIndex, settings)
				iterations = append(iterations, iteration)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
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
