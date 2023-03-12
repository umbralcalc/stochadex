package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestWienerProcess(t *testing.T) {
	t.Run(
		"test that the wiener process runs",
		func(t *testing.T) {
			settings := simulator.NewLoadSettingsConfigFromYaml("wiener_process_config.yaml")
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.StateWidths {
				iterations = append(
					iterations,
					NewWienerProcessIteration(settings.Seeds[partitionIndex]),
				)
			}
			implementations := &simulator.LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantNoMemoryTimestepFunction{
					Stepsize: 1.0,
				},
			}
			config := simulator.NewStochadexConfig(
				settings,
				implementations,
			)
			coordinator := simulator.NewPartitionCoordinator(config)
			coordinator.Run()
		},
	)
}
