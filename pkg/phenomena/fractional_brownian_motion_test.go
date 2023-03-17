package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestFractionalBrownianMotion(t *testing.T) {
	t.Run(
		"test that the fractional Brownian motion runs",
		func(t *testing.T) {
			settings := simulator.NewLoadSettingsConfigFromYaml(
				"fractional_brownian_motion_config.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.StateWidths {
				iterations = append(
					iterations,
					NewFractionalBrownianMotionIteration(
						settings.Seeds[partitionIndex],
						15,
					),
				)
			}
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.LoadImplementationsConfig{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
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
