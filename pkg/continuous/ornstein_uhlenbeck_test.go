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
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.Iterations {
				iteration := &OrnsteinUhlenbeckIteration{}
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
		"test that the Ornstein-Uhlenbeck process runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./ornstein_uhlenbeck_settings.yaml")
			iterations := make([]simulator.Iteration, 0)
			for range settings.Iterations {
				iteration := &OrnsteinUhlenbeckIteration{}
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
	t.Run(
		"exact Gaussian OU step runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"./ornstein_uhlenbeck_exact_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&OrnsteinUhlenbeckExactGaussianIteration{},
			}
			iterations[0].Configure(0, settings)
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 50,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 0.1},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
