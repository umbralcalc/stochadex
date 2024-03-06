package phenomena

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
				"hawkes_process_config.yaml",
			)
			iterations := make([][]simulator.Iteration, 0)
			serialIterations := make([]simulator.Iteration, 0)
			intensityIteration := &HawkesProcessIntensityIteration{
				excitingKernel: &kernels.ExponentialIntegrationKernel{},
			}
			intensityIteration.Configure(0, settings)
			serialIterations = append(serialIterations, intensityIteration)
			hawkesIteration := &HawkesProcessIteration{}
			hawkesIteration.Configure(1, settings)
			serialIterations = append(serialIterations, hawkesIteration)
			iterations = append(iterations, serialIterations)
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.VariableStoreOutputFunction{Store: store},
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
}
