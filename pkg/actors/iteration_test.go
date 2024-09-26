package actors

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestActorIteration(t *testing.T) {
	t.Run(
		"test that the actor iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("iteration_settings.yaml")
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &continuous.WienerProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ActorIteration{
						Iteration: &continuous.WienerProcessIteration{},
						Actor:     &AdditiveActor{},
					},
					ParamsFromUpstreamPartition: map[string]int{"action": 0},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &continuous.WienerProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ActorIteration{
						Iteration: &continuous.WienerProcessIteration{},
						Actor:     &MultiplicativeActor{},
					},
					ParamsFromUpstreamPartition: map[string]int{"action": 2},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &continuous.WienerProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ActorIteration{
						Iteration: &continuous.WienerProcessIteration{},
						Actor:     &ReplacementActor{},
					},
					ParamsFromUpstreamPartition: map[string]int{"action": 4},
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
