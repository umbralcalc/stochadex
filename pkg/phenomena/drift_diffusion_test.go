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
			partitions := make([]simulator.Partition, 0)
			driftsIteration := &simulator.ConstantValuesIteration{}
			driftsIteration.Configure(0, settings)
			diffusionsIteration := &simulator.ConstantValuesIteration{}
			diffusionsIteration.Configure(1, settings)
			iteration := &DriftDiffusionIteration{}
			iteration.Configure(2, settings)
			partitions = append(
				partitions,
				simulator.Partition{Iteration: driftsIteration},
			)
			partitions = append(
				partitions,
				simulator.Partition{Iteration: diffusionsIteration},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: iteration,
					ParamsByUpstreamPartition: map[int]string{
						0: "drift_coefficients",
						1: "diffusion_coefficients",
					},
				},
			)
			driftsIterationTwo := &simulator.ConstantValuesIteration{}
			driftsIterationTwo.Configure(3, settings)
			diffusionsIterationTwo := &simulator.ConstantValuesIteration{}
			diffusionsIterationTwo.Configure(4, settings)
			iterationTwo := &DriftDiffusionIteration{}
			iterationTwo.Configure(5, settings)
			partitions = append(
				partitions,
				simulator.Partition{Iteration: driftsIterationTwo},
			)
			partitions = append(
				partitions,
				simulator.Partition{Iteration: diffusionsIterationTwo},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: iterationTwo,
					ParamsByUpstreamPartition: map[int]string{
						3: "drift_coefficients",
						4: "diffusion_coefficients",
					},
				},
			)
			store := make([][][]float64, len(settings.StateWidths))
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
