package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestBernoulliProcessIteration(t *testing.T) {
	t.Run(
		"test that the bernoulli process works",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"bernoulli_process_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &BernoulliProcessIteration{},
				},
			)
			for index, partition := range partitions {
				partition.Iteration.Configure(index, settings)
			}
			implementations := &simulator.Implementations{
				Partitions:      partitions,
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
}
