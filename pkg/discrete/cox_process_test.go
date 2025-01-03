package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCoxProcess(t *testing.T) {
	t.Run(
		"test that the Cox process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"cox_process_settings.yaml",
			)
			// this implements a Neyman-Scott process
			rateIteration := &PoissonProcessIteration{}
			rateIteration.Configure(0, settings)
			coxIteration := &CoxProcessIteration{}
			coxIteration.Configure(1, settings)
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					rateIteration,
					coxIteration,
				},
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
		"test that the Cox process runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"cox_process_settings.yaml",
			)
			// this implements a Neyman-Scott process
			rateIteration := &PoissonProcessIteration{}
			coxIteration := &CoxProcessIteration{}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations: []simulator.Iteration{
					rateIteration,
					coxIteration,
				},
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
