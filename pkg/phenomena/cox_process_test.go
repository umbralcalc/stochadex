package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCoxProcess(t *testing.T) {
	t.Run(
		"test that the Cox process runs",
		func(t *testing.T) {
			settings := simulator.NewLoadSettingsConfigFromYaml(
				"cox_process_config.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			coxIteration := &CoxProcessIteration{}
			coxIteration.Configure(0, settings)
			iterations = append(iterations, coxIteration)
			// this implements a Neyman-Scott process
			rateIteration := &PoissonProcessIteration{}
			rateIteration.Configure(1, settings)
			iterations = append(iterations, rateIteration)
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
