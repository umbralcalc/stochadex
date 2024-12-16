package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestGeometricBrownianMotion(t *testing.T) {
	t.Run(
		"test that the geometric Brownian motion runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"geometric_brownian_motion_settings.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.Iterations {
				iteration := &GeometricBrownianMotionIteration{}
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
	t.Run(
		"test that the geometric Brownian motion runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"geometric_brownian_motion_settings.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			for range settings.Iterations {
				iteration := &GeometricBrownianMotionIteration{}
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
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
