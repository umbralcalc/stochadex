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
			partitions := make([]simulator.Partition, 0)
			intensityIteration := &HawkesProcessIntensityIteration{
				excitingKernel: &kernels.ExponentialIntegrationKernel{},
			}
			intensityIteration.Configure(0, settings)
			partitions = append(partitions, simulator.Partition{Iteration: intensityIteration})
			hawkesIteration := &HawkesProcessIteration{}
			hawkesIteration.Configure(1, settings)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: hawkesIteration,
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"intensity": {Upstream: 0},
					},
				},
			)
			store := simulator.NewVariableStore()
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
