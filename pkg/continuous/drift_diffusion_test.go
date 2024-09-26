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
			partitions := make([]simulator.Partition, 0)
			driftsIteration := &general.ConstantValuesIteration{}
			driftsIteration.Configure(0, settings)
			diffusionsIteration := &general.ConstantValuesIteration{}
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
					ParamsFromUpstreamPartition: map[string]int{
						"drift_coefficients":     0,
						"diffusion_coefficients": 1,
					},
				},
			)
			driftsIterationTwo := &general.ConstantValuesIteration{}
			driftsIterationTwo.Configure(3, settings)
			diffusionsIterationTwo := &general.ConstantValuesIteration{}
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
					ParamsFromUpstreamPartition: map[string]int{
						"drift_coefficients":     3,
						"diffusion_coefficients": 4,
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
