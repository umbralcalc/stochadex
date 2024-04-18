package params

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/phenomena"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestParamsReaderIteration(t *testing.T) {
	t.Run(
		"test that the params reader iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("reader_config.yaml")
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &simulator.ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ParamsReaderIteration{
						Iteration: &phenomena.PoissonProcessIteration{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"param_values": 0,
					},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &simulator.ConstantValuesIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &ParamsReaderIteration{
						Iteration: &phenomena.PoissonProcessIteration{},
					},
					ParamsFromUpstreamPartition: map[string]int{
						"param_values": 2,
					},
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
