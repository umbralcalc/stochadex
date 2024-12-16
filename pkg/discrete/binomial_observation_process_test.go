package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestBinomialObservationProcessIteration(t *testing.T) {
	t.Run(
		"test that the binomial observation process works",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"binomial_observation_process_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&PoissonProcessIteration{},
				&BinomialObservationProcessIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
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
		"test that the binomial observation process works with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"binomial_observation_process_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&PoissonProcessIteration{},
				&BinomialObservationProcessIteration{},
			}
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
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
