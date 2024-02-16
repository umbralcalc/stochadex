package phenomena

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestDriftDiffusionProcess(t *testing.T) {
	t.Run(
		"test that the general drift-diffusion process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("drift_diffusion_config.yaml")
			iterations := make([][]simulator.Iteration, 0)
			driftsIteration := &simulator.ConstantValuesIteration{}
			driftsIteration.Configure(0, settings)
			diffusionsIteration := &simulator.ConstantValuesIteration{}
			diffusionsIteration.Configure(1, settings)
			iteration := &DriftDiffusionIteration{}
			iteration.Configure(2, settings)
			iterations = append(
				iterations,
				[]simulator.Iteration{
					driftsIteration,
					diffusionsIteration,
					iteration,
				},
			)
			driftsIteration = &simulator.ConstantValuesIteration{}
			driftsIteration.Configure(3, settings)
			diffusionsIteration = &simulator.ConstantValuesIteration{}
			diffusionsIteration.Configure(4, settings)
			iteration = &DriftDiffusionIteration{}
			iteration.Configure(5, settings)
			iterations = append(
				iterations,
				[]simulator.Iteration{
					driftsIteration,
					diffusionsIteration,
					iteration,
				},
			)
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
