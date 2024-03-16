package actors

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestActorIteration(t *testing.T) {
	t.Run(
		"test that the actor iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("iteration_config.yaml")
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &phenomena.WienerProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ActorIteration{
						Iteration: &phenomena.WienerProcessIteration{},
						Actor:     &AdditiveActor{},
					},
					ParamsByUpstreamPartition: map[int]string{0: "action"},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &phenomena.WienerProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ActorIteration{
						Iteration: &phenomena.WienerProcessIteration{},
						Actor:     &MultiplicativeActor{},
					},
					ParamsByUpstreamPartition: map[int]string{2: "action"},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &phenomena.WienerProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ActorIteration{
						Iteration: &phenomena.WienerProcessIteration{},
						Actor:     &ReplacementActor{},
					},
					ParamsByUpstreamPartition: map[int]string{4: "action"},
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
