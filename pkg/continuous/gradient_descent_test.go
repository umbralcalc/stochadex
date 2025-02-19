package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestGradientDescent(t *testing.T) {
	t.Run(
		"test that the gradient descent process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./gradient_descent_settings.yaml")
			iterations := make([]simulator.Iteration, 0)
			gradDescentIteration := &GradientDescentIteration{}
			gradDescentIteration.Configure(0, settings)
			iterations = append(iterations, gradDescentIteration)
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
		"test that the gradient descent process runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./gradient_descent_settings.yaml")
			iterations := make([]simulator.Iteration, 0)
			gradDescentIteration := &GradientDescentIteration{}
			gradDescentIteration.Configure(0, settings)
			iterations = append(iterations, gradDescentIteration)
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
