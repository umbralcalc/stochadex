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
			partitions := make([]simulator.Partition, 0)
			// this implements a Neyman-Scott process
			rateIteration := &PoissonProcessIteration{}
			rateIteration.Configure(0, settings)
			partitions = append(partitions, simulator.Partition{Iteration: rateIteration})
			coxIteration := &CoxProcessIteration{}
			coxIteration.Configure(1, settings)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: coxIteration,
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"rates": {Upstream: 0},
					},
				},
			)
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
}
