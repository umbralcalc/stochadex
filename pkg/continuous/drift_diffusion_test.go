package continuous

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestDriftDiffusionProcess(t *testing.T) {
	t.Run(
		"test that the general drift-diffusion process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./drift_diffusion_settings.yaml")
			iterations := make([]simulator.Iteration, 0)
			driftsIteration := &general.ConstantValuesIteration{}
			driftsIteration.Configure(0, settings)
			diffusionsIteration := &general.ConstantValuesIteration{}
			diffusionsIteration.Configure(1, settings)
			iteration := &DriftDiffusionIteration{}
			iteration.Configure(2, settings)
			iterations = append(iterations, driftsIteration)
			iterations = append(iterations, diffusionsIteration)
			iterations = append(iterations, iteration)
			driftsIterationTwo := &general.ConstantValuesIteration{}
			driftsIterationTwo.Configure(3, settings)
			diffusionsIterationTwo := &general.ConstantValuesIteration{}
			diffusionsIterationTwo.Configure(4, settings)
			iterationTwo := &DriftDiffusionIteration{}
			iterationTwo.Configure(5, settings)
			iterations = append(iterations, driftsIterationTwo)
			iterations = append(iterations, diffusionsIterationTwo)
			iterations = append(iterations, iterationTwo)
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
		"test that the general drift-diffusion process runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./drift_diffusion_settings.yaml")
			iterations := make([]simulator.Iteration, 0)
			driftsIteration := &general.ConstantValuesIteration{}
			diffusionsIteration := &general.ConstantValuesIteration{}
			iteration := &DriftDiffusionIteration{}
			iterations = append(iterations, driftsIteration)
			iterations = append(iterations, diffusionsIteration)
			iterations = append(iterations, iteration)
			driftsIterationTwo := &general.ConstantValuesIteration{}
			diffusionsIterationTwo := &general.ConstantValuesIteration{}
			iterationTwo := &DriftDiffusionIteration{}
			iterations = append(iterations, driftsIterationTwo)
			iterations = append(iterations, diffusionsIterationTwo)
			iterations = append(iterations, iterationTwo)
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
