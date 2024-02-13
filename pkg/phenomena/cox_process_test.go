package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestCoxProcess(t *testing.T) {
	t.Run(
		"test that the Cox process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"cox_process_config.yaml",
			)
			iterations := make([][]simulator.Iteration, 0)
			serialIterations := make([]simulator.Iteration, 0)
			// this implements a Neyman-Scott process
			rateIteration := &PoissonProcessIteration{}
			rateIteration.Configure(0, settings)
			serialIterations = append(serialIterations, rateIteration)
			coxIteration := &CoxProcessIteration{}
			coxIteration.Configure(1, settings)
			serialIterations = append(serialIterations, coxIteration)
			iterations = append(iterations, serialIterations)
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
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
