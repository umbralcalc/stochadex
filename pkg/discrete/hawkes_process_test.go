package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/kernels"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestHawkesProcess(t *testing.T) {
	t.Run(
		"test that the Hawkes process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"hawkes_process_settings.yaml",
			)
			intensityIteration := &HawkesProcessIntensityIteration{
				excitingKernel: &kernels.ExponentialIntegrationKernel{},
			}
			intensityIteration.Configure(0, settings)
			hawkesIteration := &HawkesProcessIteration{}
			hawkesIteration.Configure(1, settings)
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					intensityIteration,
					hawkesIteration,
				},
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 250,
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
		"test that the Hawkes process runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"hawkes_process_settings.yaml",
			)
			intensityIteration := &HawkesProcessIntensityIteration{
				excitingKernel: &kernels.ExponentialIntegrationKernel{},
			}
			hawkesIteration := &HawkesProcessIteration{}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					intensityIteration,
					hawkesIteration,
				},
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 250,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}
