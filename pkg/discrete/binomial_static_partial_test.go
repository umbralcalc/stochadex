package discrete

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestBinomialStaticPartialStateObservationIteration(t *testing.T) {
	t.Run(
		"test that the binomial static partial state observation works",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"binomial_static_partial_settings.yaml",
			)
			partitions := make([]simulator.Partition, 0)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration: &PoissonProcessIteration{},
				},
			)
			partitions = append(
				partitions,
				simulator.Partition{
					Iteration:                   &BinomialStaticPartialStateObservationIteration{},
					ParamsFromUpstreamPartition: map[string]int{"observed_values": 0},
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
